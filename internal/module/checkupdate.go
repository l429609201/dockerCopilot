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

// CheckStatus 单个镜像的更新检查状态。
type CheckStatus string

const (
	// StatusLatest 已是最新（本地 digest 与远端一致）。
	StatusLatest CheckStatus = "latest"
	// StatusNeedUpdate 有更新（本地 digest 与远端不一致）。
	StatusNeedUpdate CheckStatus = "needUpdate"
	// StatusUnknown 检测未完成/失败（token 失败、网络错误、无 RepoDigests 等）。
	// 关键：失败必须显式标记为未知，而不是留空——留空会被前端当成"最新"，
	// 造成"检测失败却显示 0 个待更新"的假象。
	StatusUnknown CheckStatus = "unknown"
)

// ImageCheckList 检查更新处理后的镜像列表
type ImageCheckList struct {
	NeedUpdate bool
	// Status 明确区分 最新/有更新/未知，供前端与日志分级展示。
	Status CheckStatus
	// Reason 未知/失败时的原因（如 auth failed、no repoDigests），便于排查。
	Reason string
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
	// tokenCache 缓存 registry 拉取 token，key 为 registry host + repository path。
	// 每个镜像每轮原本都要走 challenge + token 两次往返（auth.docker.io 经常慢），
	// 同一仓库在 TTL 内复用同一 token 可显著减少往返、加快整轮检查。
	tokenCache map[string]tokenCacheEntry
}

// digestCacheEntry 单条 digest 缓存。只缓存成功结果，失败不缓存以便下轮立即重试。
type digestCacheEntry struct {
	digest   string
	cachedAt time.Time
}

// tokenCacheEntry 单条 token 缓存。只缓存成功获取的非空 token。
type tokenCacheEntry struct {
	token    string
	cachedAt time.Time
}

const ContentDigestHeader = "Docker-Content-Digest"

const (
	// digestCacheTTL 远端 digest 缓存有效期。取值需明显小于最小检查周期，
	// 保证「用户手动点检查更新」能拿到较新结果，同时挡住同一轮内的重复请求。
	digestCacheTTL = 5 * time.Minute
	// tokenCacheTTL registry token 缓存有效期。registry 签发的 token 通常有效期
	// 数分钟（Docker Hub 约 5 分钟），这里取略小值，保证复用期间 token 不会过期失效。
	tokenCacheTTL = 4 * time.Minute
	// checkConcurrency 单轮检查的并发度。并行发起 registry 请求，
	// 上限避免大量镜像时打爆 registry 速率限制或本地连接数。
	checkConcurrency = 8
	// digestHTTPTimeout 单次 manifest HEAD 请求超时。
	// 直连 Docker Hub 网络较慢、多架构镜像 manifest 拉取偏慢时，20s 易误超时，
	// 故放宽至 60s，减少 context deadline exceeded 类误报。
	digestHTTPTimeout = 60 * time.Second
)

func NewImageCheck() *ImageUpdateData {
	return &ImageUpdateData{
		Data:        map[string]ImageCheckList{},
		digestCache: map[string]digestCacheEntry{},
		tokenCache:  map[string]tokenCacheEntry{},
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

// lookupTokenCache 读取未过期的 token 缓存。
func (i *ImageUpdateData) lookupTokenCache(key string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	e, ok := i.tokenCache[key]
	if !ok || time.Since(e.cachedAt) > tokenCacheTTL {
		return "", false
	}
	return e.token, true
}

// storeTokenCache 写入 token 缓存，只缓存非空 token，并顺带清理过期条目。
func (i *ImageUpdateData) storeTokenCache(key, token string) {
	if token == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.tokenCache == nil {
		i.tokenCache = map[string]tokenCacheEntry{}
	}
	now := time.Now()
	for k, v := range i.tokenCache {
		if now.Sub(v.cachedAt) > tokenCacheTTL {
			delete(i.tokenCache, k)
		}
	}
	i.tokenCache[key] = tokenCacheEntry{token: token, cachedAt: now}
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

// CheckUpdate 执行一轮镜像更新检查（无进度上报，供定时检查与启动检查使用）。
func (i *ImageUpdateData) CheckUpdate(imageList []types.Image) {
	i.CheckUpdateWithProgress(imageList, nil)
}

// CheckProgress 单轮检查的进度回调参数。
// done/total 为已完成/待检查镜像数，current 为刚检查完的镜像名（便于前端显示"正在检查 xxx"）。
type CheckProgress struct {
	Done    int
	Total   int
	Current string
}

// CheckUpdateWithProgress 执行一轮镜像更新检查，并在每个镜像检查完成后回调上报进度。
// 通过 beginCheck/endCheck 去重，避免启动检查与定时检查并发执行；
// 检查过程中出现的单镜像错误不会清空已有结果，保证前端状态稳定。
//
// 镜像间以固定并发度并行检查：原实现串行遍历，镜像数量多时单轮耗时随数量线性增长。
//
// onProgress 可为 nil（定时/启动检查不需要进度）。返回值表示本轮是否真正执行：
// false 说明已有检查在进行中，本轮被跳过，调用方据此给出对应提示而非显示一个空任务。
func (i *ImageUpdateData) CheckUpdateWithProgress(imageList []types.Image, onProgress func(CheckProgress)) bool {
	if !i.beginCheck() {
		logx.Info("已有镜像更新检查在进行中，跳过本轮")
		return false
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
	total := len(targets)
	if total == 0 {
		if onProgress != nil {
			onProgress(CheckProgress{Done: 0, Total: 0})
		}
		return true
	}

	workers := checkConcurrency
	if total < workers {
		workers = total
	}
	logx.Infof("开始镜像更新检查：%d 个镜像，并发度 %d", total, workers)
	start := time.Now()

	// 并发 worker 共同推进完成计数，故用独立互斥锁保护回调，
	// 避免多个 goroutine 同时进入回调导致进度写入竞争。
	var progressMu sync.Mutex
	done := 0
	report := func(name string) {
		if onProgress == nil {
			return
		}
		progressMu.Lock()
		done++
		snapshot := CheckProgress{Done: done, Total: total, Current: name}
		progressMu.Unlock()
		onProgress(snapshot)
	}

	tasks := make(chan types.Image)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 单个镜像 panic 不应带崩整轮检查
			for image := range tasks {
				func(img types.Image) {
					// 无论检查成功、失败还是 panic，都要推进进度，否则进度会卡住不到 100%
					defer func() {
						if r := recover(); r != nil {
							logx.Errorf("检查镜像 %s:%s 时 panic 已恢复: %v", img.ImageName, img.ImageTag, r)
						}
						report(img.ImageName + ":" + img.ImageTag)
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

	logx.Infof("镜像更新检查完成：%d 个镜像，耗时 %s", total, time.Since(start).Round(time.Millisecond))
	return true
}

func (i *ImageUpdateData) checkSingleImage(image types.Image) {
	imageRef := image.ImageName + ":" + image.ImageTag

	// markUnknown 统一记录"检测未完成/失败"结果。
	// 关键修复：以前这些分支直接 return 不写结果，map 里没有该 imageID，
	// 前端 NeedUpdate 读到 (false,false) 会当成"最新"，导致失败被吞成 0 个待更新。
	// 现在显式写入 StatusUnknown，让失败可见、可区分。
	markUnknown := func(reason string) {
		i.setResult(image.ID, ImageCheckList{NeedUpdate: false, Status: StatusUnknown, Reason: reason})
	}

	// ImageName/ImageTag 解析失败的镜像（悬空镜像等）无法构造 manifest URL，直接跳过（不标未知，本就无意义）
	if image.ImageName == "None" || image.ImageTag == "None" {
		logx.Debugf("跳过无有效 tag 的镜像: %s (ID: %s)", imageRef, image.ID)
		return
	}

	digestURL, err := BuildManifestURL(image)
	if err != nil {
		logx.Errorf("构造 manifest URL 失败 [%s]: %v", imageRef, err)
		markUnknown("构造 manifest URL 失败: " + err.Error())
		return
	}

	// 优先命中缓存，省掉 token 获取 + manifest HEAD 两次网络往返
	remoteDigest, cached := i.lookupDigestCache(digestURL)
	if !cached {
		// token 缓存 key 用「registry host + repository path」（不含 tag），
		// 同一仓库不同 tag 的 pull token 通用，可跨镜像/跨轮复用。
		tokenKey := tokenCacheKey(image)
		token, tokenCached := i.lookupTokenCache(tokenKey)
		if !tokenCached {
			// 部分 registry 无需 token 即可读 manifest，取不到不算失败，继续尝试匿名请求
			var errToken error
			token, errToken = GetToken(image, "")
			if errToken != nil {
				logx.Debugf("获取 token 失败或无需 token [%s]: %v", imageRef, errToken)
			}
			// 只缓存成功获取的非空 token（storeTokenCache 内部已对空值忽略）
			i.storeTokenCache(tokenKey, token)
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
			markUnknown("获取远端 digest 失败: " + err.Error())
			return
		}
		i.storeDigestCache(digestURL, remoteDigest)
	}

	if len(image.RepoDigests) == 0 {
		// 本地构建、未推送过的镜像没有 RepoDigests，无法比对，属正常情况
		logx.Debugf("本地无 repoDigest，跳过比对 [%s]", imageRef)
		markUnknown("本地无 repoDigest，无法比对")
		return
	}
	if remoteDigest == "" {
		logx.Errorf("远端返回的 digest 为空 [%s]", imageRef)
		markUnknown("远端返回的 digest 为空")
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
	status := StatusLatest
	if needUpdate {
		// 有更新是需要用户关注的结论，保持 Info
		logx.Infof("镜像有更新 [%s] remoteDigest=%s localRepoDigests=%v",
			imageRef, remoteDigest, image.RepoDigests)
		status = StatusNeedUpdate
	} else {
		// 已是最新占绝大多数，降为 Debug，避免每轮刷屏
		logx.Debugf("镜像已是最新 [%s]", imageRef)
	}
	// 并发安全写入，禁止直接操作 map
	i.setResult(image.ID, ImageCheckList{NeedUpdate: needUpdate, Status: status})
}

// tokenCacheKey 生成 token 缓存 key：registry host + repository path（不含 tag）。
// 同一仓库不同 tag 的 pull token 通用，用此 key 可最大化复用。
// 解析失败时回退用 ImageName，保证不同镜像不会误共享 token。
func tokenCacheKey(image types.Image) string {
	normalizedRef, err := ref.ParseNormalizedNamed(image.ImageName)
	if err != nil {
		return image.ImageName
	}
	host, _ := GetRegistryAddress(normalizedRef.Name())
	return host + "/" + ref.Path(normalizedRef)
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
	// Accept 头顺序决定 registry 返回哪一层的 Docker-Content-Digest。
	// 多架构镜像本地 RepoDigests 存的是「manifest list / OCI index」的 digest，
	// 因此必须把 list/index 排在单架构 manifest 之前，否则 Docker Hub 会返回
	// 单架构 manifest 的 digest，与本地索引 digest 永远不相等，导致误判。
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.oci.image.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")

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
