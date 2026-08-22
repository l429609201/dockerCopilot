package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// composeFilePattern 允许的 compose 文件名后缀。
var allowedComposeNames = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

// SafeResolveDir 校验并解析项目目录，确保其位于某个扫描根目录内，防止路径穿越。
// 返回解析后的绝对路径及其所属根目录。
func SafeResolveDir(scanPaths []string, projectDir string) (resolved string, root string, err error) {
	if len(scanPaths) == 0 {
		return "", "", fmt.Errorf("未配置 Compose 扫描目录")
	}
	cleaned := filepath.Clean(projectDir)
	for _, base := range scanPaths {
		baseClean := filepath.Clean(base)
		// 目标必须等于根目录或位于根目录之下
		rel, relErr := filepath.Rel(baseClean, cleaned)
		if relErr != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return cleaned, baseClean, nil
		}
	}
	return "", "", fmt.Errorf("目录不在允许的扫描范围内")
}

// SafeResolveFile 在已校验的项目目录内解析文件名，禁止子目录穿越，仅允许白名单文件名。
func SafeResolveFile(projectDir, filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("非法文件名")
	}
	full := filepath.Clean(filepath.Join(projectDir, name))
	prefix := filepath.Clean(projectDir) + string(os.PathSeparator)
	if full != filepath.Clean(projectDir) && !strings.HasPrefix(full, prefix) {
		return "", fmt.Errorf("非法文件名")
	}
	return full, nil
}

// IsComposeFileName 判断文件名是否为受支持的 compose 文件。
func IsComposeFileName(name string) bool {
	lower := strings.ToLower(name)
	for _, candidate := range allowedComposeNames {
		if lower == candidate {
			return true
		}
	}
	return false
}

// CheckFileSize 校验文件大小不超过上限。
func CheckFileSize(size, max int64) error {
	if max > 0 && size > max {
		return fmt.Errorf("文件大小 %d 超过上限 %d", size, max)
	}
	return nil
}
