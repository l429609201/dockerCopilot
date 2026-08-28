package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/compose"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// ComposeLogic 承载 Compose 项目的列表、文件读写、校验与部署编排。
type ComposeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewComposeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ComposeLogic {
	return &ComposeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// scanPaths 读取生效的扫描根目录：优先动态配置(AppConfig)，为空回退静态 yaml。
func (l *ComposeLogic) scanPaths() []string {
	if dyn := l.svcCtx.AppConfig.Get().Compose.ScanPaths; len(dyn) > 0 {
		return dyn
	}
	return l.svcCtx.Config.Compose.ScanPaths
}

// maxDepth 生效扫描深度：动态优先，<=0 回退静态 yaml。
func (l *ComposeLogic) maxDepth() int {
	if d := l.svcCtx.AppConfig.Get().Compose.MaxDepth; d > 0 {
		return d
	}
	return l.svcCtx.Config.Compose.MaxDepth
}

// maxFileSize 生效文件大小上限：动态优先，<=0 回退静态 yaml。
func (l *ComposeLogic) maxFileSize() int64 {
	if s := l.svcCtx.AppConfig.Get().Compose.MaxFileSize; s > 0 {
		return s
	}
	return l.svcCtx.Config.Compose.MaxFileSize
}

// commandTimeoutSec 生效命令超时：动态优先，<=0 回退静态 yaml。
func (l *ComposeLogic) commandTimeoutSec() int {
	if t := l.svcCtx.AppConfig.Get().Compose.CommandTimeoutSec; t > 0 {
		return t
	}
	return l.svcCtx.Config.Compose.CommandTimeoutSec
}

// allowHighRisk 生效高风险开关：一旦前端保存过配置(Configured)则以动态为准，否则回退静态。
func (l *ComposeLogic) allowHighRisk() bool {
	c := l.svcCtx.AppConfig.Get().Compose
	if c.Configured {
		return c.AllowHighRisk
	}
	return l.svcCtx.Config.Compose.AllowHighRisk
}

// GetConfig 返回当前生效的 Compose 配置及是否已在前端配置，供前端配置卡片回显。
func (l *ComposeLogic) GetConfig() (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	dyn := l.svcCtx.AppConfig.Get().Compose
	resp.Data = map[string]interface{}{
		"scanPaths":         l.scanPaths(),
		"maxDepth":          l.maxDepth(),
		"maxFileSize":       l.maxFileSize(),
		"commandTimeoutSec": l.commandTimeoutSec(),
		"allowHighRisk":     l.allowHighRisk(),
		// configured 标记是否已在前端保存过（区分回退静态 yaml 的情况）
		"configured": dyn.Configured,
	}
	return resp, nil
}

// SaveConfig 保存 Compose 扫描配置到持久化 AppConfig；对路径做安全校验后即时生效。
func (l *ComposeLogic) SaveConfig(req *types.ComposeConfigReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	// 清洗并校验扫描路径：必须为绝对路径且真实存在的目录，防止误配与注入
	cleaned := make([]string, 0, len(req.ScanPaths))
	for _, raw := range req.ScanPaths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return bad(resp, "扫描路径必须为绝对路径："+p), nil
		}
		clean := filepath.Clean(p)
		info, statErr := os.Stat(clean)
		if statErr != nil || !info.IsDir() {
			return bad(resp, "扫描路径不存在或不是目录："+clean), nil
		}
		cleaned = append(cleaned, clean)
	}

	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		cfg.Compose.ScanPaths = cleaned
		cfg.Compose.MaxDepth = req.MaxDepth
		cfg.Compose.MaxFileSize = req.MaxFileSize
		cfg.Compose.CommandTimeoutSec = req.CommandTimeoutSec
		cfg.Compose.AllowHighRisk = req.AllowHighRisk
		cfg.Compose.Configured = true
		return nil
	})
	if updateErr != nil {
		return bad(resp, "保存失败："+updateErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{"scanPaths": cleaned}
	return resp, nil
}

// List 扫描并返回所有 Compose 项目。
func (l *ComposeLogic) List() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	paths := l.scanPaths()
	if len(paths) == 0 {
		resp.Code = 400
		resp.Msg = "未配置 Compose 扫描目录(Compose.ScanPaths)"
		resp.Data = []interface{}{}
		return resp, nil
	}
	scanner := compose.NewScanner(paths, l.maxDepth())
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = scanner.Scan()
	return resp, nil
}

// ReadFile 读取指定项目下的指定文件内容。
func (l *ComposeLogic) ReadFile(req *types.ComposeFileReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	dir, decErr := compose.DecodeID(req.ID)
	if decErr != nil {
		return bad(resp, "非法项目ID"), nil
	}
	resolvedDir, _, sErr := compose.SafeResolveDir(l.scanPaths(), dir)
	if sErr != nil {
		return bad(resp, sErr.Error()), nil
	}
	filePath, fErr := compose.SafeResolveFile(resolvedDir, req.Filename)
	if fErr != nil {
		return bad(resp, fErr.Error()), nil
	}
	info, statErr := os.Stat(filePath)
	if statErr != nil {
		return bad(resp, "文件不存在"), nil
	}
	if szErr := compose.CheckFileSize(info.Size(), l.maxFileSize()); szErr != nil {
		return bad(resp, szErr.Error()), nil
	}
	content, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return bad(resp, "读取失败："+readErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"filename": req.Filename,
		"content":  string(content),
	}
	return resp, nil
}

// Validate 校验传入的 compose 内容并返回风险警告。
func (l *ComposeLogic) Validate(req *types.ComposeValidateReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	resp.Data = compose.Validate([]byte(req.Content))
	return resp, nil
}

// SaveFile 校验并写入 compose 文件内容；写入前做 YAML 语法校验，写入前备份原文件。
func (l *ComposeLogic) SaveFile(req *types.ComposeFileSaveReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	dir, decErr := compose.DecodeID(req.ID)
	if decErr != nil {
		return bad(resp, "非法项目ID"), nil
	}
	resolvedDir, _, sErr := compose.SafeResolveDir(l.scanPaths(), dir)
	if sErr != nil {
		return bad(resp, sErr.Error()), nil
	}
	filePath, fErr := compose.SafeResolveFile(resolvedDir, req.Filename)
	if fErr != nil {
		return bad(resp, fErr.Error()), nil
	}
	// 写入前做语法校验，避免写入损坏的 compose 文件
	vr := compose.Validate([]byte(req.Content))
	if !vr.Valid {
		return bad(resp, "内容校验失败："+vr.Error), nil
	}
	// 备份原文件（若存在）
	if old, readErr := os.ReadFile(filePath); readErr == nil {
		_ = os.WriteFile(filePath+".bak", old, 0644)
	}
	if wErr := os.WriteFile(filePath, []byte(req.Content), 0644); wErr != nil {
		return bad(resp, "写入失败："+wErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{"warnings": vr.Warnings}
	return resp, nil
}

// Browse 浏览 DC 自身文件系统的目录，仅返回子目录（只读，供目录选择器使用）。
// 安全策略：仅接受绝对路径并清理后使用；出错时返回可读提示而非 500。
func (l *ComposeLogic) Browse(req *types.ComposeBrowseReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}

	// path 为空则从根目录开始（容器内为 "/"）
	target := strings.TrimSpace(req.Path)
	if target == "" {
		target = string(os.PathSeparator)
	}
	// 必须为绝对路径，避免相对路径带来的歧义与越权
	if !filepath.IsAbs(target) {
		return bad(resp, "路径必须为绝对路径"), nil
	}
	// 清理 . 与 .. ，规整为标准路径
	target = filepath.Clean(target)

	info, statErr := os.Stat(target)
	if statErr != nil {
		return bad(resp, "无法访问该目录："+statErr.Error()), nil
	}
	if !info.IsDir() {
		return bad(resp, "该路径不是目录"), nil
	}

	entries, readErr := os.ReadDir(target)
	if readErr != nil {
		return bad(resp, "读取目录失败："+readErr.Error()), nil
	}

	// 收集子目录和文件（忽略隐藏项）
	dirs := make([]string, 0)
	files := make([]string, 0)
	for _, e := range entries {
		name := e.Name()
		// 忽略隐藏文件/目录（以 . 开头）
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, name)
		} else {
			files = append(files, name)
		}
	}

	// 父目录：已在根目录时 parent 为空，前端据此隐藏"上一级"
	parent := filepath.Dir(target)
	if parent == target {
		parent = ""
	}

	resp.Data = map[string]interface{}{
		"path":   target,
		"parent": parent,
		"dirs":   dirs,
		"files":  files,
	}
	return resp, nil
}

// bad 填充业务错误响应的通用辅助。
func bad(resp *types.Resp, msg string) *types.Resp {
	resp.Code = 400
	resp.Msg = msg
	resp.Data = map[string]interface{}{}
	return resp
}

// CreateFolder 在指定目录下创建文件夹。
func (l *ComposeLogic) CreateFolder(req *types.ComposeCreateFolderReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}

	// 清理路径，防止路径穿越
	parentPath := filepath.Clean(req.ParentPath)
	folderName := filepath.Clean(req.FolderName)

	// 校验：父路径必须是绝对路径
	if !filepath.IsAbs(parentPath) {
		return bad(resp, "父路径必须为绝对路径"), nil
	}

	// 校验：文件夹名不能包含路径分隔符
	if strings.Contains(folderName, string(os.PathSeparator)) {
		return bad(resp, "文件夹名称不能包含路径分隔符"), nil
	}

	// 拼接完整路径
	fullPath := filepath.Join(parentPath, folderName)

	// 检查是否已存在
	if _, err := os.Stat(fullPath); err == nil {
		return bad(resp, "文件夹已存在"), nil
	}

	// 创建文件夹
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return bad(resp, "创建文件夹失败："+err.Error()), nil
	}

	logx.Infof("已创建文件夹: %s", fullPath)
	resp.Data = map[string]interface{}{"path": fullPath}
	return resp, nil
}

// CreateComposeFile 在指定目录下创建 Compose 配置文件（带模板内容）。
func (l *ComposeLogic) CreateComposeFile(req *types.ComposeCreateFileReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}

	// 清理路径
	parentPath := filepath.Clean(req.ParentPath)
	fileName := filepath.Clean(req.FileName)

	// 校验：父路径必须是绝对路径
	if !filepath.IsAbs(parentPath) {
		return bad(resp, "父路径必须为绝对路径"), nil
	}

	// 校验：文件名不能包含路径分隔符
	if strings.Contains(fileName, string(os.PathSeparator)) {
		return bad(resp, "文件名不能包含路径分隔符"), nil
	}

	// 拼接完整路径
	fullPath := filepath.Join(parentPath, fileName)

	// 检查是否已存在
	if _, err := os.Stat(fullPath); err == nil {
		return bad(resp, "文件已存在"), nil
	}

	// 默认模板内容
	templateContent := `version: '3.8'

services:
  app:
    image: nginx:latest
    container_name: my-app
    ports:
      - "8080:80"
    volumes:
      - ./data:/usr/share/nginx/html
    restart: unless-stopped
    networks:
      - default

networks:
  default:
    driver: bridge
`

	// 写入文件
	if err := os.WriteFile(fullPath, []byte(templateContent), 0644); err != nil {
		return bad(resp, "创建文件失败："+err.Error()), nil
	}

	logx.Infof("已创建 Compose 文件: %s", fullPath)
	resp.Data = map[string]interface{}{"path": fullPath}
	return resp, nil
}

// ReadFileByPath 读取指定路径的文件内容（文件管理器用）。
func (l *ComposeLogic) ReadFileByPath(req *types.ComposeReadFileReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}

	// 清理路径
	filePath := filepath.Clean(req.Path)

	// 校验：必须是绝对路径
	if !filepath.IsAbs(filePath) {
		return bad(resp, "文件路径必须为绝对路径"), nil
	}

	// 检查文件是否存在
	info, err := os.Stat(filePath)
	if err != nil {
		return bad(resp, "文件不存在："+err.Error()), nil
	}
	if info.IsDir() {
		return bad(resp, "该路径是目录，不是文件"), nil
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return bad(resp, "读取文件失败："+err.Error()), nil
	}

	resp.Data = map[string]interface{}{
		"path":    filePath,
		"content": string(content),
	}
	return resp, nil
}

// SaveFileByPath 保存文件内容到指定路径（文件管理器用）。
func (l *ComposeLogic) SaveFileByPath(req *types.ComposeSaveFileReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}

	// 清理路径
	filePath := filepath.Clean(req.Path)

	// 校验：必须是绝对路径
	if !filepath.IsAbs(filePath) {
		return bad(resp, "文件路径必须为绝对路径"), nil
	}

	// 写入文件
	if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
		return bad(resp, "保存文件失败："+err.Error()), nil
	}

	logx.Infof("已保存文件: %s", filePath)
	return resp, nil
}
