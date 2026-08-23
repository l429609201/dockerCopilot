package utiles

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/docker/docker/api/types/registry"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
)

// RegistryIDAuto 是"自适应凭证"的约定值：规则 RegistryID 为该值时，
// 更新每个容器时按其镜像所属 registry 自动匹配凭证，匹配不到则匿名拉取。
const RegistryIDAuto = "auto"

// EncodeRegistryAuth 将凭据编码为 Docker SDK 需要的 base64(JSON) 形式，
// 用于 ImagePull 的 RegistryAuth 字段。空凭据返回空串（匿名拉取）。
func EncodeRegistryAuth(cred *appconfig.RegistryCredential) (string, error) {
	if cred == nil || cred.Username == "" {
		return "", nil
	}
	authConfig := registry.AuthConfig{
		Username:      cred.Username,
		Password:      cred.Password,
		ServerAddress: cred.Registry,
	}
	encoded, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encoded), nil
}


// parseImageRegistryHost 从镜像引用中解析出 registry host。
// ghcr.io/user/app:tag -> ghcr.io；nginx:latest -> docker.io。
func parseImageRegistryHost(image string) string {
	if image == "" {
		return "docker.io"
	}
	first := image
	if i := strings.IndexByte(image, '/'); i >= 0 {
		first = image[:i]
	} else {
		return "docker.io"
	}
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first
	}
	return "docker.io"
}

// normalizeRegistryHost 归一化 registry 地址用于比较：
// 去掉协议前缀和结尾斜杠，空串视为 docker.io（Docker Hub）。
func normalizeRegistryHost(reg string) string {
	r := strings.TrimSpace(reg)
	r = strings.TrimPrefix(r, "https://")
	r = strings.TrimPrefix(r, "http://")
	r = strings.TrimSuffix(r, "/")
	if r == "" || r == "index.docker.io" || r == "registry-1.docker.io" {
		return "docker.io"
	}
	return r
}

// MatchRegistryAuthByImage 按镜像所属 registry 自动匹配已保存的凭证并编码。
// 匹配不到（或匹配到的凭证无用户名）时返回空串，表示匿名拉取。
func MatchRegistryAuthByImage(cfg *appconfig.Store, image string) string {
	if cfg == nil {
		return ""
	}
	host := parseImageRegistryHost(image)
	for _, cred := range cfg.ListRegistries() {
		if normalizeRegistryHost(cred.Registry) == host {
			if auth, err := EncodeRegistryAuth(&cred); err == nil {
				return auth
			}
			return ""
		}
	}
	return ""
}