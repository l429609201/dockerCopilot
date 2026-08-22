package compose

import (
	"os"
	"path/filepath"

	"github.com/zeromicro/go-zero/core/logx"
)

// Project 表示一个被发现的 Compose 项目。
type Project struct {
	ID          string   `json:"id"`          // base64(目录绝对路径) 作为稳定ID
	Name        string   `json:"name"`        // 目录名作为项目名
	Dir         string   `json:"dir"`         // 目录绝对路径
	ComposeFile string   `json:"composeFile"` // 主 compose 文件名
	Files       []string `json:"files"`       // 目录下的 compose 文件名列表
}

// Scanner 负责在配置的扫描根目录内发现 Compose 项目。
type Scanner struct {
	scanPaths []string
	maxDepth  int
}

// NewScanner 创建扫描器。
func NewScanner(scanPaths []string, maxDepth int) *Scanner {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	return &Scanner{scanPaths: scanPaths, maxDepth: maxDepth}
}

// Scan 遍历所有扫描根目录，返回发现的 Compose 项目列表。
func (s *Scanner) Scan() []Project {
	var projects []Project
	seen := make(map[string]struct{})
	for _, base := range s.scanPaths {
		baseClean := filepath.Clean(base)
		s.walk(baseClean, baseClean, 0, seen, &projects)
	}
	return projects
}

// walk 递归遍历目录，深度受 maxDepth 限制。
func (s *Scanner) walk(base, dir string, depth int, seen map[string]struct{}, out *[]Project) {
	if depth > s.maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		logx.Errorf("扫描 Compose 目录失败 %s: %v", dir, err)
		return
	}
	var composeFiles []string
	for _, e := range entries {
		if !e.IsDir() && IsComposeFileName(e.Name()) {
			composeFiles = append(composeFiles, e.Name())
		}
	}
	if len(composeFiles) > 0 {
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			*out = append(*out, Project{
				ID:          EncodeID(dir),
				Name:        filepath.Base(dir),
				Dir:         dir,
				ComposeFile: primaryComposeFile(composeFiles),
				Files:       composeFiles,
			})
		}
	}
	// 继续向子目录递归
	for _, e := range entries {
		if e.IsDir() {
			s.walk(base, filepath.Join(dir, e.Name()), depth+1, seen, out)
		}
	}
}

// primaryComposeFile 选出主 compose 文件（优先 docker-compose.yml）。
func primaryComposeFile(files []string) string {
	priority := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
	for _, p := range priority {
		for _, f := range files {
			if f == p {
				return f
			}
		}
	}
	if len(files) > 0 {
		return files[0]
	}
	return ""
}
