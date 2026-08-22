package module

import (
	"crypto/tls"
	"errors"
	"fmt"
	ref "github.com/distribution/reference"
	"github.com/onlyLTY/dockerCopilot/internal/types"
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
}

const ContentDigestHeader = "Docker-Content-Digest"

func NewImageCheck() *ImageUpdateData {
	return &ImageUpdateData{
		Data: map[string]ImageCheckList{},
	}
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
func (i *ImageUpdateData) CheckUpdate(imageList []types.Image) {
	if !i.beginCheck() {
		logx.Info("已有镜像更新检查在进行中，跳过本轮")
		return
	}
	defer i.endCheck()
	for _, image := range imageList {
		if strings.Contains(image.ImageName, "0nlylty/dockercopilot") {
			continue
		}
		i.checkSingleImage(image)
	}
}

func (i *ImageUpdateData) checkSingleImage(image types.Image) {
	token, err := GetToken(image, "")
	if err != nil {
		logx.Error("获取token失败或者无需获取token，继续尝试检查" + err.Error())
	}
	digestURL, err := BuildManifestURL(image)
	if err != nil {
		logx.Error("获取digestURL失败" + err.Error())
		return
	}
	remoteDigest, err := GetDigest(digestURL, token)
	if err != nil {
		logx.Error("获取digest失败" + err.Error())
		return
	}
	if len(image.RepoDigests) == 0 {
		logx.Error("未在本地获取到repoDigest" + image.ImageName + ":" + image.ImageTag)
		return
	}
	needUpdate := false
	for _, localRepoDigests := range image.RepoDigests {
		localDigest := strings.Split(localRepoDigests, "@")[1]
		if remoteDigest != localDigest {
			if remoteDigest == "" || localDigest == "" {
				logx.Error("Digest为空" + image.ImageName + ":" + image.ImageTag)
				continue
			}
			logx.Info(image.ImageName + ":" + image.ImageTag + " need update")
			logx.Infof("localDigest: %s, remoteDigest: %s", localDigest, remoteDigest)
			needUpdate = true
		} else {
			logx.Info(image.ImageName + ":" + image.ImageTag + " not need update")
			needUpdate = false
		}
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

func GetDigest(url string, token string) (string, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("HEAD", url, nil)

	if token != "" {
		req.Header.Add("Authorization", token)
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	res, err := client.Do(req)
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
		return "", fmt.Errorf("registry responded to head request with %q, auth: %q", res.Status, wwwAuthHeader)
	}
	return res.Header.Get(ContentDigestHeader), nil
}
