package compose

import "encoding/base64"

// EncodeID 将目录绝对路径编码为 URL 安全的稳定项目ID。
func EncodeID(dir string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(dir))
}

// DecodeID 将项目ID还原为目录绝对路径。
func DecodeID(id string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
