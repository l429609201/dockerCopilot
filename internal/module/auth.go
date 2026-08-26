package module

import (
	"encoding/json"
	"errors"
	"fmt"
	ref "github.com/distribution/reference"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ChallengeHeader = "WWW-Authenticate"
const (
	DefaultRegistryDomain = "docker.io"
	DefaultRegistryHost   = "index.docker.io"
)

// authHTTPTimeout 认证类请求的超时。原实现用裸 http.Client{}（无超时），
// registry 无响应时会永久挂住，并行检查下会持续占用 worker。
const authHTTPTimeout = 15 * time.Second

func GetToken(image types.Image, registryAuth string) (string, error) {
	// 每个镜像每轮都会走到这里，用 Debug 级别避免刷屏掩盖真实错误
	logx.Debugf("获取 token，镜像: %s", image.ImageName)
	normalizedRef, err := ref.ParseNormalizedNamed(image.ImageName)
	if err != nil {
		return "", err
	}

	URL := GetChallengeURL(normalizedRef)

	var req *http.Request
	if req, err = GetChallengeRequest(URL); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: authHTTPTimeout}
	var res *http.Response
	if res, err = client.Do(req); err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("GetToken关闭Body失败" + err.Error())
		}
	}(res.Body)
	v := res.Header.Get(ChallengeHeader)

	challenge := strings.ToLower(v)
	if strings.HasPrefix(challenge, "basic") {
		if registryAuth == "" {
			return "", fmt.Errorf("no credentials available")
		}

		return fmt.Sprintf("Basic %s", registryAuth), nil
	}
	if strings.HasPrefix(challenge, "bearer") {
		return GetBearerHeader(challenge, normalizedRef, registryAuth)
	}

	return "", errors.New("unsupported challenge type from registry")
}

func GetChallengeRequest(URL url.URL) (*http.Request, error) {
	req, err := http.NewRequest("GET", URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Watchtower (Docker)")
	return req, nil
}

func GetBearerHeader(challenge string, imageRef ref.Named, registryAuth string) (string, error) {
	client := http.Client{Timeout: authHTTPTimeout}
	authURL, err := GetAuthURL(challenge, imageRef)

	if err != nil {
		return "", err
	}

	var r *http.Request
	if r, err = http.NewRequest("GET", authURL.String(), nil); err != nil {
		return "", err
	}

	if registryAuth != "" {
		r.Header.Add("Authorization", fmt.Sprintf("Basic %s", registryAuth))
		logx.Debug("使用已配置凭证请求 token")
	} else {
		// 公开镜像匿名拉取本就无需凭证，这不是错误，降为 Debug 避免噪音
		logx.Debug("未配置 registry 凭证，按匿名方式获取 token")
	}

	var authResponse *http.Response
	if authResponse, err = client.Do(r); err != nil {
		return "", err
	}

	body, _ := io.ReadAll(authResponse.Body)
	tokenResponse := &types.TokenResponse{}

	err = json.Unmarshal(body, tokenResponse)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Bearer %s", tokenResponse.Token), nil
}

func GetAuthURL(challenge string, imageRef ref.Named) (*url.URL, error) {
	loweredChallenge := strings.ToLower(challenge)
	raw := strings.TrimPrefix(loweredChallenge, "bearer")

	pairs := strings.Split(raw, ",")
	values := make(map[string]string, len(pairs))

	for _, pair := range pairs {
		trimmed := strings.Trim(pair, " ")
		if key, val, ok := strings.Cut(trimmed, "="); ok {
			values[key] = strings.Trim(val, `"`)
		}
	}
	if values["realm"] == "" || values["service"] == "" {

		return nil, fmt.Errorf("challenge header did not include all values needed to construct an auth url")
	}

	authURL, _ := url.Parse(values["realm"])
	q := authURL.Query()
	q.Add("service", values["service"])

	scopeImage := ref.Path(imageRef)

	scope := fmt.Sprintf("repository:%s:pull", scopeImage)
	q.Add("scope", scope)

	authURL.RawQuery = q.Encode()
	return authURL, nil
}

func GetChallengeURL(imageRef ref.Named) url.URL {
	host, _ := GetRegistryAddress(imageRef.Name())

	URL := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/v2/",
	}
	return URL
}

// GetRegistryAddress 解析镜像引用所属的 registry 地址。
//
// 只使用镜像自身声明的源站，不做任何加速站替换或可达性探测：
//   - 镜像名显式带 registry（如 ghcr.io/x/y、docker.1ms.run/x/y）时原样返回，
//     用户想走哪个镜像源就在镜像名里指定；
//   - 未带 registry 的官方镜像（如 nginx:latest）归一到 Docker Hub 源站。
//
// 旧实现会串行探测 index.docker.io + 10 个加速站（每个 5s 超时），
// 且每个镜像检查要走两遍，镜像多时整轮检查极慢；同时自动改写镜像源
// 也让用户无法确定 digest 究竟来自哪个站点，故一并移除。
func GetRegistryAddress(imageRef string) (string, error) {
	normalizedRef, err := ref.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "", err
	}

	address := ref.Domain(normalizedRef)
	if address == DefaultRegistryDomain {
		address = DefaultRegistryHost
	}
	return address, nil
}
