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

// bad 填充业务错误响应的通用辅助。
func bad(resp *types.Resp, msg string) *types.Resp {
	resp.Code = 400
	resp.Msg = msg
	resp.Data = map[string]interface{}{}
	return resp
}
