package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
)

// DockerHubRateLimit 描述 Docker Hub 的拉取次数配额。
type DockerHubRateLimit struct {
	Limit     int    // 周期内总配额（如 200）
	Remaining int    // 剩余次数
	Source    string // authenticated（已登录凭据）/ anonymous（匿名按 IP）
}

// dockerHub 速率探测使用的固定探针仓库与鉴权端点。
const (
	dhTokenURL    = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:ratelimitpreview/test:pull"
	dhManifestURL = "https://registry-1.docker.io/v2/ratelimitpreview/test/manifests/latest"
)

// CheckDockerHubRateLimit 使用给定凭据探测 Docker Hub 剩余拉取次数。
// cred 为空或用户名为空时按匿名（IP 维度）探测。
// 仅 Docker Hub 具备该配额机制，其他 registry 不适用。
func CheckDockerHubRateLimit(ctx context.Context, cred *appconfig.RegistryCredential) (*DockerHubRateLimit, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// 第一步：换取匿名/登录 token
	token, source, err := fetchDockerHubToken(ctx, client, cred)
	if err != nil {
		return nil, err
	}

	// 第二步：对探针镜像发起 HEAD 请求，读取速率响应头
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, dhManifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// 指定 manifest 媒体类型，避免因内容协商导致的异常
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limit := parseRateHeader(resp.Header.Get("RateLimit-Limit"))
	remaining := parseRateHeader(resp.Header.Get("RateLimit-Remaining"))

	// 未返回速率头，通常表示该账户不受限（如 Pro/Team 付费账户）
	if resp.Header.Get("RateLimit-Limit") == "" && resp.Header.Get("RateLimit-Remaining") == "" {
		return &DockerHubRateLimit{Limit: -1, Remaining: -1, Source: source}, nil
	}

	return &DockerHubRateLimit{Limit: limit, Remaining: remaining, Source: source}, nil
}

// fetchDockerHubToken 获取访问探针镜像所需的 Bearer token。
// 有凭据时带 Basic Auth 换取"已登录"配额，否则为匿名配额。
func fetchDockerHubToken(ctx context.Context, client *http.Client, cred *appconfig.RegistryCredential) (token, source string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dhTokenURL, nil)
	if err != nil {
		return "", "", err
	}
	source = "anonymous"
	if cred != nil && cred.Username != "" {
		req.SetBasicAuth(cred.Username, cred.Password)
		source = "authenticated"
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", fmt.Errorf("凭据校验失败（401），请检查用户名或访问令牌")
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("获取 Docker Hub token 失败，状态码 %d", resp.StatusCode)
	}

	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	t := body.Token
	if t == "" {
		t = body.AccessToken
	}
	if t == "" {
		return "", "", fmt.Errorf("未获取到有效 token")
	}
	return t, source, nil
}

// parseRateHeader 解析形如 "200;w=21600" 的速率头，取分号前的数字。
func parseRateHeader(v string) int {
	if v == "" {
		return 0
	}
	if i := strings.IndexByte(v, ';'); i >= 0 {
		v = v[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return n
}
