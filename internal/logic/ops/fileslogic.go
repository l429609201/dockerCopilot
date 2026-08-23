package ops

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// splitPath 将完整文件路径拆分为父目录与文件名。
func splitPath(full string) (dir, name string) {
	full = strings.TrimRight(full, "/")
	dir = path.Dir(full)
	name = path.Base(full)
	return
}

// stringReader 将字符串包装为 io.Reader。
func stringReader(s string) io.Reader { return strings.NewReader(s) }

// FilesLogic 处理容器内文件管理（列目录、读写、增删改）。
// 所有路径由 containerops 层统一做防穿越校验。
type FilesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	ops    *containerops.Service
}

func NewFilesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FilesLogic {
	return &FilesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		ops:    containerops.New(svcCtx),
	}
}

// List 列出目录内容。
func (l *FilesLogic) List(req *types.FileListReq) (*types.Resp, error) {
	resp := &types.Resp{}
	entries, err := l.ops.ListFiles(l.ctx, req.Id, req.Path)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{"path": req.Path, "entries": entries}
	return resp, nil
}

// Read 读取文本文件用于预览/编辑。
func (l *FilesLogic) Read(req *types.FileReadReq) (*types.Resp, error) {
	resp := &types.Resp{}
	fc, err := l.ops.ReadTextFile(l.ctx, req.Id, req.Path)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = fc
	return resp, nil
}

// Write 保存文本内容到文件（新建或覆盖）。
func (l *FilesLogic) Write(req *types.FileWriteReq) (*types.Resp, error) {
	resp := &types.Resp{}
	dir, name := splitPath(req.Path)
	if err := l.ops.UploadFile(l.ctx, req.Id, dir, name, stringReader(req.Content), 0); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 写入文件 %s（%d 字节）", req.Id, req.Path, len(req.Content))
	resp.Code = 200
	resp.Msg = "success"
	return resp, nil
}

// Mkdir 新建目录。
func (l *FilesLogic) Mkdir(req *types.FileMkdirReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.Mkdir(l.ctx, req.Id, req.Path, req.Name); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 在 %s 下创建目录 %s", req.Id, req.Path, req.Name)
	resp.Code = 200
	resp.Msg = "success"
	return resp, nil
}

// Delete 删除文件/目录。
func (l *FilesLogic) Delete(req *types.FileDeleteReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.RemovePath(l.ctx, req.Id, req.Path); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 删除路径 %s", req.Id, req.Path)
	resp.Code = 200
	resp.Msg = "success"
	return resp, nil
}

// Rename 重命名/移动文件。
func (l *FilesLogic) Rename(req *types.FileRenameReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.RenameFile(l.ctx, req.Id, req.Src, req.Dst); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 重命名 %s -> %s", req.Id, req.Src, req.Dst)
	resp.Code = 200
	resp.Msg = "success"
	return resp, nil
}