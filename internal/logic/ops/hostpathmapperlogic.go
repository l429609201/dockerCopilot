package ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
)

// HostPathMapperLogic 宿主机路径映射逻辑。
type HostPathMapperLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHostPathMapperLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HostPathMapperLogic {
	return &HostPathMapperLogic{ctx: ctx, svcCtx: svcCtx}
}

// ResolveHostPath 将容器内路径解析为宿主机路径。
// 例如：/compose/nginx/conf -> /home/root_nas/docker/nginx/conf
func (l *HostPathMapperLogic) ResolveHostPath(containerPath string) (string, error) {
	cfg := l.svcCtx.AppConfig.Get()
	if !cfg.HostPathMapper.Enabled {
		return "", fmt.Errorf("宿主机路径映射功能未启用")
	}

	// 规范化路径
	containerPath = filepath.Clean(containerPath)

	// 遍历映射规则，找到最长匹配的前缀
	var bestMatch *struct {
		ContainerPath string
		HostPath      string
	}
	maxLen := 0
	for i := range cfg.HostPathMapper.Mappings {
		mapping := &cfg.HostPathMapper.Mappings[i]
		cleanContainerPath := filepath.Clean(mapping.ContainerPath)
		if strings.HasPrefix(containerPath, cleanContainerPath) {
			if len(cleanContainerPath) > maxLen {
				bestMatch = &struct {
					ContainerPath string
					HostPath      string
				}{
					ContainerPath: mapping.ContainerPath,
					HostPath:      mapping.HostPath,
				}
				maxLen = len(cleanContainerPath)
			}
		}
	}

	if bestMatch == nil {
		return "", fmt.Errorf("未找到匹配的路径映射规则: %s", containerPath)
	}

	// 拼接宿主机路径
	relativePath := strings.TrimPrefix(containerPath, filepath.Clean(bestMatch.ContainerPath))
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))
	hostPath := filepath.Join(bestMatch.HostPath, relativePath)

	return hostPath, nil
}

// HostFileEntry 宿主机文件条目（简化版，兼容前端文件管理器）。
type HostFileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	IsDir   bool   `json:"isDir"`
}

// ListMappedDir 列出映射目录的内容（宿主机路径，通过容器路径访问）。
func (l *HostPathMapperLogic) ListMappedDir(containerPath string) (*types.Resp, error) {
	cfg := l.svcCtx.AppConfig.Get()
	if !cfg.HostPathMapper.Enabled {
		return nil, fmt.Errorf("宿主机路径映射功能未启用")
	}

	// 规范化路径
	containerPath = filepath.Clean(containerPath)

	// 解析为宿主机路径
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return nil, err
	}

	// 读取宿主机目录
	entries, err := os.ReadDir(hostPath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	// 构造响应（兼容前端文件管理器格式）
	items := make([]HostFileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		items = append(items, HostFileEntry{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
			IsDir:   entry.IsDir(),
		})
	}

	// 排序：目录在前，然后按名称
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return &types.Resp{
		Code: 200,
		Msg:  "success",
		Data: map[string]interface{}{
			"path":    containerPath,
			"entries": items,
		},
	}, nil
}

// GetMappings 获取所有路径映射配置。
func (l *HostPathMapperLogic) GetMappings() *types.Resp {
	cfg := l.svcCtx.AppConfig.Get()
	return &types.Resp{
		Code: 200,
		Msg:  "success",
		Data: map[string]interface{}{
			"enabled":  cfg.HostPathMapper.Enabled,
			"mappings": cfg.HostPathMapper.Mappings,
		},
	}
}

// ReadFile 读取文本文件内容。
func (l *HostPathMapperLogic) ReadFile(containerPath string) (*types.Resp, error) {
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	content, err := os.ReadFile(hostPath)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "读取文件失败"}, err
	}

	return &types.Resp{
		Code: 200,
		Msg:  "success",
		Data: map[string]interface{}{
			"path":    containerPath,
			"content": string(content),
		},
	}, nil
}

// WriteFile 写入文本文件内容。
func (l *HostPathMapperLogic) WriteFile(containerPath, content string) (*types.Resp, error) {
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	err = os.WriteFile(hostPath, []byte(content), 0644)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "写入文件失败"}, err
	}

	return &types.Resp{Code: 200, Msg: "文件保存成功"}, nil
}

// CreateDir 创建目录。
func (l *HostPathMapperLogic) CreateDir(containerPath string) (*types.Resp, error) {
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	err = os.MkdirAll(hostPath, 0755)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "创建目录失败"}, err
	}

	return &types.Resp{Code: 200, Msg: "目录创建成功"}, nil
}

// DeletePath 删除文件或目录。
func (l *HostPathMapperLogic) DeletePath(containerPath string) (*types.Resp, error) {
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	err = os.RemoveAll(hostPath)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "删除失败"}, err
	}

	return &types.Resp{Code: 200, Msg: "删除成功"}, nil
}

// RenamePath 重命名或移动文件/目录。
func (l *HostPathMapperLogic) RenamePath(oldContainerPath, newContainerPath string) (*types.Resp, error) {
	oldHostPath, err := l.ResolveHostPath(oldContainerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	newHostPath, err := l.ResolveHostPath(newContainerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	err = os.Rename(oldHostPath, newHostPath)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "重命名失败"}, err
	}

	return &types.Resp{Code: 200, Msg: "重命名成功"}, nil
}

// DownloadFile 下载文件（返回文件内容和文件名）。
func (l *HostPathMapperLogic) DownloadFile(containerPath string) ([]byte, string, error) {
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return nil, "", err
	}

	content, err := os.ReadFile(hostPath)
	if err != nil {
		return nil, "", fmt.Errorf("读取文件失败: %w", err)
	}

	filename := filepath.Base(hostPath)
	return content, filename, nil
}

// UploadFile 上传文件。
func (l *HostPathMapperLogic) UploadFile(containerPath, filename string, file io.Reader) (*types.Resp, error) {
	// 解析目标目录
	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{Code: 400, Msg: err.Error()}, err
	}

	// 确保目标是目录
	info, err := os.Stat(hostPath)
	if err != nil {
		return &types.Resp{Code: 404, Msg: "目标路径不存在"}, err
	}
	if !info.IsDir() {
		return &types.Resp{Code: 400, Msg: "目标路径不是目录"}, fmt.Errorf("不是目录")
	}

	// 创建目标文件
	targetPath := filepath.Join(hostPath, filename)
	outFile, err := os.Create(targetPath)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "创建文件失败"}, err
	}
	defer outFile.Close()

	// 复制内容
	_, err = io.Copy(outFile, file)
	if err != nil {
		return &types.Resp{Code: 500, Msg: "写入文件失败"}, err
	}

	return &types.Resp{Code: 200, Msg: "文件上传成功"}, nil
}

