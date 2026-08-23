package containerops

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// FileEntry 表示容器内一个文件/目录条目。
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`     // 权限字符串，如 drwxr-xr-x
	ModTime string `json:"modTime"`  // 修改时间（原始展示）
	IsDir   bool   `json:"isDir"`
	IsLink  bool   `json:"isLink"`
	Link    string `json:"link,omitempty"` // 软链接目标
}

// sanitizePath 校验并归一化容器内的绝对路径，防止路径穿越与注入。
//   - 必须以 '/' 开头（绝对路径）；
//   - 不允许出现空字节和控制字符；
//   - path.Clean 归一化后必须仍是绝对路径，且不含 ".." 段。
//
// 返回归一化后的安全路径；非法输入返回错误。
func sanitizePath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	// 拒绝空字节与控制字符（含换行、回车），防止注入与截断
	for _, r := range p {
		if r == 0 || r == '\n' || r == '\r' {
			return "", fmt.Errorf("路径包含非法字符")
		}
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("路径必须为绝对路径")
	}
	cleaned := path.Clean(p)
	// Clean 后若仍含 ".." 段（理论上不会，双保险）或不以 / 开头，拒绝
	if cleaned != "/" && strings.HasSuffix(cleaned, "/") {
		cleaned = strings.TrimRight(cleaned, "/")
	}
	if !strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("非法路径")
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径不允许包含 '..'")
		}
	}
	return cleaned, nil
}

// joinChild 在已校验的父目录下拼接子名称，并再次校验，防止子名称越界。
func joinChild(dir, name string) (string, error) {
	if strings.ContainsAny(name, "/\x00\n\r") {
		return "", fmt.Errorf("名称包含非法字符")
	}
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("名称非法")
	}
	return sanitizePath(path.Join(dir, name))
}

// ListFiles 列出目录内容。使用 exec `ls` 数组传参 + `--` 终止选项，防注入。
func (s *Service) ListFiles(ctx context.Context, id, dir string) ([]FileEntry, error) {
	safe, err := sanitizePath(dir)
	if err != nil {
		return nil, err
	}
	// -A 显示隐藏文件（不含 . ..），-l 详情，-Q 名称加引号，--time-style 统一时间
	res, err := s.Exec(ctx, id, []string{"ls", "-Al", "-Q", "--time-style=long-iso", "--", safe}, "", "", 30, 512*1024)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("列目录失败: %s", strings.TrimSpace(res.Output))
	}
	return parseLsOutput(res.Output), nil
}

// parseLsOutput 解析 `ls -Al -Q --time-style=long-iso` 的输出为结构化条目。
func parseLsOutput(out string) []FileEntry {
	entries := make([]FileEntry, 0)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		e, ok := parseLsLine(line)
		if ok {
			entries = append(entries, e)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // 目录在前
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries
}

// parseLsLine 解析单行 ls 详情。格式示例：
// drwxr-xr-x 2 root root 4096 2024-08-19 15:38 "app"
func parseLsLine(line string) (FileEntry, bool) {
	// 前 5 个空白分隔字段：mode links owner group size；随后是 日期 时间；最后是引号包裹的名称
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return FileEntry{}, false
	}
	mode := fields[0]
	size, _ := strconv.ParseInt(fields[4], 10, 64)
	modTime := fields[5] + " " + fields[6]
	// 名称在第一个 '"' 到最后一个 '"' 之间（可能含空格）
	name := ""
	if i := strings.Index(line, "\""); i >= 0 {
		if j := strings.LastIndex(line, "\""); j > i {
			name = line[i+1 : j]
		}
	}
	if name == "" {
		return FileEntry{}, false
	}
	isDir := strings.HasPrefix(mode, "d")
	isLink := strings.HasPrefix(mode, "l")
	link := ""
	if isLink {
		if idx := strings.Index(name, " -> "); idx >= 0 {
			link = name[idx+4:]
			name = name[:idx]
			link = strings.Trim(link, "\"")
			name = strings.Trim(name, "\"")
		}
	}
	return FileEntry{Name: name, Size: size, Mode: mode, ModTime: modTime, IsDir: isDir, IsLink: isLink, Link: link}, true
}

// Mkdir 在容器内创建目录（-p 递归）。
func (s *Service) Mkdir(ctx context.Context, id, dir, name string) error {
	safeParent, err := sanitizePath(dir)
	if err != nil {
		return err
	}
	target, err := joinChild(safeParent, name)
	if err != nil {
		return err
	}
	res, err := s.Exec(ctx, id, []string{"mkdir", "-p", "--", target}, "", "", 30, 64*1024)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("创建目录失败: %s", strings.TrimSpace(res.Output))
	}
	return nil
}

// RemovePath 删除容器内的文件或目录（-rf）。危险操作，调用方需已确认。
func (s *Service) RemovePath(ctx context.Context, id, target string) error {
	safe, err := sanitizePath(target)
	if err != nil {
		return err
	}
	// 禁止删除根目录，避免灾难
	if safe == "/" {
		return fmt.Errorf("禁止删除根目录")
	}
	res, err := s.Exec(ctx, id, []string{"rm", "-rf", "--", safe}, "", "", 60, 64*1024)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("删除失败: %s", strings.TrimSpace(res.Output))
	}
	return nil
}

// RenameFile 重命名/移动容器内文件。src、dst 均校验。
func (s *Service) RenameFile(ctx context.Context, id, src, dst string) error {
	safeSrc, err := sanitizePath(src)
	if err != nil {
		return err
	}
	safeDst, err := sanitizePath(dst)
	if err != nil {
		return err
	}
	if safeSrc == "/" || safeDst == "/" {
		return fmt.Errorf("禁止操作根目录")
	}
	res, err := s.Exec(ctx, id, []string{"mv", "--", safeSrc, safeDst}, "", "", 60, 64*1024)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("重命名失败: %s", strings.TrimSpace(res.Output))
	}
	return nil
}
