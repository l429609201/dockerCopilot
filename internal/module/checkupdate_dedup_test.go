package module

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/l429609201/dockerCopilot/internal/types"
)

// countingTransport 记录 manifest 请求次数，并模拟较慢的 registry，制造并发重叠。
type countingTransport struct {
	calls atomic.Int32
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	time.Sleep(100 * time.Millisecond)
	h := http.Header{}
	h.Set(ContentDigestHeader, "sha256:remote")
	return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

// TestMultiHostSameImageSingleRegistryRequest 多台主机上的同一镜像在同一轮并发检查时，
// 只向 registry 发一次 manifest 请求，且每台主机都拿到各自的结果。
func TestMultiHostSameImageSingleRegistryRequest(t *testing.T) {
	ct := &countingTransport{}
	orig := digestHTTPClient.Transport
	digestHTTPClient.Transport = ct
	defer func() { digestHTTPClient.Transport = orig }()

	i := NewImageCheck()
	hosts := []string{"local", "remote1", "remote2", "remote3"}
	var list []types.Image
	for _, h := range hosts {
		img := types.Image{HostID: h, ImageName: "registry.test:5000/foo/bar", ImageTag: "latest"}
		// ID/RepoDigests 来自内嵌的 Docker SDK 镜像结构
		img.ID = "sha256:a"
		img.RepoDigests = []string{"registry.test:5000/foo/bar@sha256:old"}
		list = append(list, img)
	}
	// 预置 token 缓存，避免测试访问真实 token 服务
	i.storeTokenCache(tokenCacheKey(list[0]), "Bearer test")

	if !i.CheckUpdateWithProgress(list, nil) {
		t.Fatal("检查未执行")
	}
	if n := ct.calls.Load(); n != 1 {
		t.Fatalf("manifest 请求次数=%d，期望 1", n)
	}
	for _, h := range hosts {
		if need, ok := i.NeedUpdate(h, "sha256:a"); !ok || !need {
			t.Fatalf("主机 %s 结果错误: need=%v ok=%v", h, need, ok)
		}
	}
}
