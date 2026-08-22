package utiles

import (
	"encoding/base64"
	"encoding/json"

	"github.com/docker/docker/api/types/registry"
	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
)

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
