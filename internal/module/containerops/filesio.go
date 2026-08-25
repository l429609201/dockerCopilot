package containerops

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	dcontainer "github.com/docker/docker/api/types/container"
)

const (
	// MaxUploadSize 上传/写入单文件上限：100MB
	MaxUploadSize = 100 * 1024 * 1024
	// MaxPreviewSize 在线预览文本上限：1MB
	MaxPreviewSize = 1 * 1024 * 1024
)

// FileContent 承载读取到的文件内容与元信息。
type FileContent struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Content string `json:"content"`
	Truncated bool `json:"truncated"` // 是否因超限被截断
}

// DownloadFile 从容器复制文件，返回原始 tar 流中的单个文件读取器与文件名。
// 调用方负责关闭返回的 ReadCloser（此处已读入内存，返回 bytes.Reader 更安全）。
func (s *Service) DownloadFile(ctx context.Context, id, target string) (string, []byte, error) {
	safe, err := sanitizePath(target)
	if err != nil {
		return "", nil, err
	}
	cli, err := s.cliOrErr()
	if err != nil {
		return "", nil, err
	}
	rc, stat, err := cli.CopyFromContainer(ctx, id, safe)
	if err != nil {
		return "", nil, fmt.Errorf("读取容器文件失败: %w", err)
	}
	defer rc.Close()
	if stat.Mode.IsDir() {
		return "", nil, fmt.Errorf("不支持下载目录，请先打包")
	}

	tr := tar.NewReader(rc)
	name := path.Base(safe)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("解析文件流失败: %w", err)
		}
		// 安全校验：tar 条目名不得逃逸（防恶意 daemon 返回穿越条目）
		if !isSafeTarName(hdr.Name) {
			return "", nil, fmt.Errorf("文件流包含非法条目")
		}
		if hdr.Typeflag == tar.TypeReg {
			// 限制读入内存的大小
			limited := io.LimitReader(tr, MaxUploadSize+1)
			data, err := io.ReadAll(limited)
			if err != nil {
				return "", nil, fmt.Errorf("读取文件内容失败: %w", err)
			}
			if int64(len(data)) > MaxUploadSize {
				return "", nil, fmt.Errorf("文件超过 %d MB 上限", MaxUploadSize/1024/1024)
			}
			return name, data, nil
		}
	}
	return "", nil, fmt.Errorf("未在文件流中找到常规文件")
}

// ReadTextFile 读取文本文件用于在线预览/编辑，超过 MaxPreviewSize 截断。
func (s *Service) ReadTextFile(ctx context.Context, id, target string) (*FileContent, error) {
	name, data, err := s.downloadLimited(ctx, id, target, MaxPreviewSize)
	if err != nil {
		return nil, err
	}
	truncated := false
	if int64(len(data)) >= MaxPreviewSize {
		truncated = true
	}
	return &FileContent{Name: name, Size: int64(len(data)), Content: string(data), Truncated: truncated}, nil
}

// downloadLimited 复制文件并最多读取 limit 字节。
func (s *Service) downloadLimited(ctx context.Context, id, target string, limit int64) (string, []byte, error) {
	safe, err := sanitizePath(target)
	if err != nil {
		return "", nil, err
	}
	cli, err := s.cliOrErr()
	if err != nil {
		return "", nil, err
	}
	rc, stat, err := cli.CopyFromContainer(ctx, id, safe)
	if err != nil {
		return "", nil, fmt.Errorf("读取容器文件失败: %w", err)
	}
	defer rc.Close()
	if stat.Mode.IsDir() {
		return "", nil, fmt.Errorf("目标是目录，无法读取内容")
	}
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("解析文件流失败: %w", err)
		}
		if !isSafeTarName(hdr.Name) {
			return "", nil, fmt.Errorf("文件流包含非法条目")
		}
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(io.LimitReader(tr, limit))
			if err != nil {
				return "", nil, fmt.Errorf("读取内容失败: %w", err)
			}
			return path.Base(safe), data, nil
		}
	}
	return "", nil, fmt.Errorf("未找到常规文件")
}

// isSafeTarName 校验 tar 条目名不含穿越段，防止解包逃逸。
func isSafeTarName(name string) bool {
	if name == "" {
		return false
	}
	clean := path.Clean("/" + name)
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// UploadFile 将内容写入容器指定目录下的文件（新建或覆盖）。
// dir 为目标目录，name 为文件名，content 为文件内容（已限制大小由调用方保证）。
func (s *Service) UploadFile(ctx context.Context, id, dir, name string, content io.Reader, mode int64) error {
	safeDir, err := sanitizePath(dir)
	if err != nil {
		return err
	}
	if _, err := joinChild(safeDir, name); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0644
	}
	// 读入内存并限制大小
	buf, err := io.ReadAll(io.LimitReader(content, MaxUploadSize+1))
	if err != nil {
		return fmt.Errorf("读取上传内容失败: %w", err)
	}
	if int64(len(buf)) > MaxUploadSize {
		return fmt.Errorf("文件超过 %d MB 上限", MaxUploadSize/1024/1024)
	}
	// 打成 tar 流
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(buf)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("构造归档失败: %w", err)
	}
	if _, err := tw.Write(buf); err != nil {
		return fmt.Errorf("写入归档失败: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("关闭归档失败: %w", err)
	}
	cli, err := s.cliOrErr()
	if err != nil {
		return err
	}
	if err := cli.CopyToContainer(ctx, id, safeDir, &tarBuf, dcontainer.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("写入容器失败: %w", err)
	}
	return nil
}
