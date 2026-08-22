package compose

import (
	"context"
	"os"

	"github.com/onlyLTY/dockerCopilot/internal/module/compose"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
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

// scanPaths 读取配置的扫描根目录。
func (l *ComposeLogic) scanPaths() []string {
	return l.svcCtx.Config.Compose.ScanPaths
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
	scanner := compose.NewScanner(paths, l.svcCtx.Config.Compose.MaxDepth)
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
	if szErr := compose.CheckFileSize(info.Size(), l.svcCtx.Config.Compose.MaxFileSize); szErr != nil {
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
