package module

import (
	"errors"
	"fmt"
	ref "github.com/distribution/reference"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"net"
	"net/http"
	url2 "net/url"
	"strings"
	"sync"
	"time"
)

// ImageCheckList 检查更新处理后的镜像列表
type ImageCheckList struct {
	NeedUpdate bool
}

// ImageUpdateData 保存镜像更新检查结果。
// 注意：Data 会被后台检查 goroutine 写入，同时被大量容器列表请求读取，
// 因此必须通过内部读写锁访问，禁止外部直接读写 map，否则会触发
// Go 运行时的 "concurrent map read and map write" 致命错误（历史卡死根因）。
type ImageUpdateData struct {
	mu   sync.RWMutex
	Data map[string]ImageCheckList
	// checking 标记后台是否正在执行一轮检查，用于避免重复触发。
	checking bool
	// digestCache 缓存远端 manifest digest，key 为 manifest URL。
	// 同一轮内多个镜像可能指向同一 URL（多主机同镜像），跨轮则在 TTL 内复用，
	// 避免对 registry 的重复 HEAD 请求（Docker Hub 有匿名速率限制）。
	digestCache map[string]digestCacheEntry
}

// digestCacheEntry 单条 digest 缓存。只缓存成功结果，失败不缓存以便下轮立即重试。
type digestCacheEntry struct {
	digest   string
	cachedAt time.Time
}

const ContentDigestHeader = "Docker-Content-Digest"

const (
	// digestCacheTTL 远端 digest 缓存有效期。取值需明显小于最小检查周期，
	// 保证「用户手动点检查更新」能拿到较新结果，同时挡住同一轮内的重复请求。
	digestCacheTTL = 5 * time.Minute
	// checkConcurrency 单轮检查的并发度。并行发起 registry 请求，
	// 上限避免大量镜像时打爆 registry 速率限制或本地连接数。
	checkConcurrency = 8
	// digestHTTPTimeout 单次 manifest HEAD 请求超时。
	digestHTTPTimeout = 20 * time.Second
)

func NewImageCheck() *ImageUpdateData {
	return &ImageUpdateData{
		Data:        map[string]ImageCheckList{},
		digestCache: map[string]digestCacheEntry{},
	}
}

// lookupDigestCache 读取未过期的 digest 缓存。
func (i *ImageUpdateData) lookupDigestCache(url string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	e, ok := i.digestCache[url]
	if !ok || time.Since(e.cachedAt) > digestCacheTTL {
		return "", false
	}
	return e.digest, true
}

// storeDigestCache 写入 digest 缓存，同时顺带清理已过期条目，避免长期运行后无界增长。
func (i *ImageUpdateData) storeDigestCache(url, digest string) {
	if digest == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.digestCache == nil {
		i.digestCache = map[string]digestCacheEntry{}
	}
	now := time.Now()
	for k, v := range i.digestCache {
		if now.Sub(v.cachedAt) > digestCacheTTL {
			delete(i.digestCache, k)
		}
	}
	i.digestCache[url] = digestCacheEntry{digest: digest, cachedAt: now}
}

// NeedUpdate 以并发安全的方式读取指定镜像ID是否需要更新。
func (i *ImageUpdateData) NeedUpdate(imageID string) (bool, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.Data[imageID]
	return v.NeedUpdate, ok
}

// Snapshot 返回当前检查结果的浅拷贝快照，供只读遍历使用，避免持锁期间执行耗时逻辑。
func (i *ImageUpdateData) Snapshot() map[string]ImageCheckList {
	i.mu.RLock()
	defer i.mu.RUnlock()
	snapshot := make(map[string]ImageCheckList, len(i.Data))
	for k, v := range i.Data {
		snapshot[k] = v
	}
	return snapshot
}

// setResult 以并发安全的方式写入单条检查结果。
func (i *ImageUpdateData) setResult(imageID string, result ImageCheckList) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Data == nil {
		i.Data = map[string]ImageCheckList{}
	}
	i.Data[imageID] = result
}

// beginCheck 尝试占用检查标记，返回 false 表示已有检查在进行中，应跳过本轮。
func (i *ImageUpdateData) beginCheck() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.checking {
		return false
	}
	i.checking = true
	return true
}

// endCheck 释放检查标记。
func (i *ImageUpdateData) endCheck() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.checking = false
}

// CheckUpdate 执行一轮镜像更新检查。
// 通过 beginCheck/endCheck 去重，避免启动检查与定时检查并发执行；
// 检查过程中出现的单镜像错误不会清空已有结果，保证前端状态稳定。
//
// 镜像间以固定并发度并行检查：原实现串行遍历，镜像数量多时单轮耗时随数量线性增长。
func (i *ImageUpdateData) CheckUpdate(imageList []types.Image) {
	if !i.beginCheck() {
		logx.Info("已有镜像更新检查在进行中，跳过本轮")
		return
	}
	defer i.endCheck()

	// 先过滤掉不需要检查的镜像（自身镜像），再分发给 worker
	targets := make([]types.Image, 0, len(imageList))
	for _, image := range imageList {
		if strings.Contains(image.ImageName, "0nlylty/dockercopilot") {
			continue
		}
		targets = append(targets, image)
	}
	if len(targets) == 0 {
		return
	}

	workers := checkConcurrency
	if len(targets) < workers {
		workers = len(targets)
	}
	logx.Infof("开始镜像更新检查：%d 个镜像，并发度 %d", len(targets), workers)
	start := time.Now()

	tasks := make(chan types.Image)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 单个镜像 panic 不应带崩整轮检查
			for image := range tasks {
				func(img types.Image) {
					defer func() {
						if r := recover(); r != nil {
							logx.Errorf("检查镜像 %s:%s 时 panic 已恢复: %v", img.ImageName, img.ImageTag, r)
						}
					}()
					i.checkSingleImage(img)
				}(image)
			}
		}()
	}
	for _, image := range targets {
		tasks <- image
	}
	close(tasks)
	wg.Wait()

	logx.Infof("镜像更新检查完成：%d 个镜像，耗时 %s", len(targets), time.Since(start).Round(time.Millisecond))
}

func (i *ImageUpdateData) checkSingleImage(image types.Image) {
	imageRef := image.ImageName + ":" + image.ImageTag

	// ImageName/ImageTag 解析失败的镜像（悬空镜像等）无法构造 manifest URL，直接跳过
	if image.ImageName == "None" || image.ImageTag == "None" {
		logx.Debugf("跳过无有效 tag 的镜像: %s (ID: %s)", imageRef, image.ID)
		return
	}

	digestURL, err := BuildManifestURL(image)
	if err != nil {
		logx.Errorf("构造 manifest URL 失败 [%s]: %v", imageRef, err)
		return
	}

	// 优先命中缓存，省掉 token 获取 + manifest HEAD 两次网络往返
	remoteDigest, cached := i.lookupDigestCache(digestURL)
	if !cached {
		// 部分 registry 无需 token 即可读 manifest，取不到不算失败，继续尝试匿名请求
		token, errToken := GetToken(image, "")
		if errToken != nil {
			logx.Debugf("获取 token 失败或无需 token [%s]: %v", imageRef, errToken)
		}
		remoteDigest, err = GetDigest(digestURL, token)
		if err != nil {
			// 401/404 是私有仓库或未配置凭据时的预期结果，不算故障，降为 Info；
			// go-zero 无 Warn 级别，故在文案中标注 warn 供前端与人工识别。
			// 这样真正的网络故障、registry 不可用才会留在 error 里。
			if isAuthOrNotFoundErr(err) {
				logx.Infof("warn: 跳过无权限或不存在的镜像 [%s]: %v", imageRef, err)
			} else {
				logx.Errorf("获取远端 digest 失败 [%s] url=%s: %v", imageRef, digestURL, err)
			}
			return
		}
		i.storeDigestCache(digestURL, remoteDigest)
	}

	if len(image.RepoDigests) == 0 {
		// 本地构建、未推送过的镜像没有 RepoDigests，无法比对，属正常情况
		logx.Debugf("本地无 repoDigest，跳过比对 [%s]", imageRef)
		return
	}
	if remoteDigest == "" {
		logx.Errorf("远端返回的 digest 为空 [%s]", imageRef)
		return
	}

	// 任一本地 digest 与远端一致即视为已是最新。
	// 原实现在循环里反复覆盖 needUpdate，多条 RepoDigests 时结论被最后一条决定，会导致误判。
	needUpdate := true
	for _, localRepoDigests := range image.RepoDigests {
		parts := strings.SplitN(localRepoDigests, "@", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		if parts[1] == remoteDigest {
			needUpdate = false
			break
		}
	}
	if needUpdate {
		// 有更新是需要用户关注的结论，保持 Info
		logx.Infof("镜像有更新 [%s] remoteDigest=%s localRepoDigests=%v",
			imageRef, remoteDigest, image.RepoDigests)
	} else {
		// 已是最新占绝大多数，降为 Debug，避免每轮刷屏
		logx.Debugf("镜像已是最新 [%s]", imageRef)
	}
	// 并发安全写入，禁止直接操作 map
	i.setResult(image.ID, ImageCheckList{NeedUpdate: needUpdate})
}

func BuildManifestURL(image types.Image) (string, error) {
	normalizedRef, err := ref.ParseDockerRef(image.ImageName + ":" + image.ImageTag)
	if err != nil {
		return "", err
	}
	normalizedTaggedRef, isTagged := normalizedRef.(ref.NamedTagged)
	if !isTagged {
		return "", errors.New("镜像无tag" + normalizedRef.String())
	}

	host, ErrGetRegistryAddress := GetRegistryAddress(normalizedTaggedRef.Name())
	img, tag := ref.Path(normalizedTaggedRef), normalizedTaggedRef.Tag()

	if ErrGetRegistryAddress != nil {
		return "", ErrGetRegistryAddress
	}

	url := url2.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("/v2/%s/manifests/%s", img, tag),
	}
	return url.String(), nil
}

// digestHTTPClient 供所有 manifest HEAD 请求共享。
// 原实现每次调用都新建 Transport，并发检查时连接池无法复用、每个镜像都要重新 TLS 握手。
var digestHTTPClient = &http.Client{
	Timeout: digestHTTPTimeout,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   checkConcurrency,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

func GetDigest(url string, token string) (string, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return "", err
	}

	if token != "" {
		req.Header.Add("Authorization", token)
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	res, err := digestHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("GetDigest关闭body失败" + err.Error())
		}
	}(res.Body)

	if res.StatusCode != 200 {
		wwwAuthHeader := res.Header.Get("www-authenticate")
		if wwwAuthHeader == "" {
			wwwAuthHeader = "not present"
		}
		return "", &RegistryStatusError{
			StatusCode: res.StatusCode,
			Status:     res.Status,
			WWWAuth:    wwwAuthHeader,
		}
	}
	return res.Header.Get(ContentDigestHeader), nil
}

// RegistryStatusError 表示 registry 返回了非 200 状态码。
// 保留状态码便于调用方按类型分级处理，避免靠错误文本做字符串匹配。
type RegistryStatusError struct {
	StatusCode int
	Status     string
	WWWAuth    string
}

func (e *RegistryStatusError) Error() string {
	return fmt.Sprintf("registry responded to head request with %q, auth: %q", e.Status, e.WWWAuth)
}

// isAuthOrNotFoundErr 判断错误是否为鉴权失败或镜像不存在。
// 这类结果在未配置凭据、使用私有仓库时属预期情况，不应记为 error。
func isAuthOrNotFoundErr(err error) bool {
	var statusErr *RegistryStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return false
	}
}
