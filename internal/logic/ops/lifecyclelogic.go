package ops

import (
	"context"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// LifecycleLogic 处理容器生命周期操作（pause/unpause/kill/remove/rename）。
type LifecycleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	ops    *containerops.Service
}

func NewLifecycleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LifecycleLogic {
	return &LifecycleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		ops:    containerops.New(svcCtx),
	}
}

// ok/fail 构造统一响应。
func ok(resp *types.Resp) *types.Resp {
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp
}
func fail(resp *types.Resp, msg string) *types.Resp {
	resp.Code = 400
	resp.Msg = msg
	resp.Data = map[string]interface{}{}
	return resp
}

// Pause 暂停容器。
func (l *LifecycleLogic) Pause(req *types.ContainerActionReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.Pause(req.Id); err != nil {
		return fail(resp, err.Error()), nil
	}
	return ok(resp), nil
}

// Unpause 恢复容器。
func (l *LifecycleLogic) Unpause(req *types.ContainerActionReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.Unpause(req.Id); err != nil {
		return fail(resp, err.Error()), nil
	}
	return ok(resp), nil
}

// Kill 强制终止容器（高风险，前端需二次确认）。
func (l *LifecycleLogic) Kill(req *types.ContainerActionReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.Kill(req.Id); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 被强制终止", req.Id)
	return ok(resp), nil
}

// Remove 删除容器（高风险，前端需二次确认）。
func (l *LifecycleLogic) Remove(req *types.ContainerRemoveReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if err := l.ops.Remove(req.Id, req.Force, req.RemoveVolumes); err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 被删除(force=%v,volumes=%v)", req.Id, req.Force, req.RemoveVolumes)
	return ok(resp), nil
}

// Rename 重命名容器，校验名称格式。
func (l *LifecycleLogic) Rename(req *types.ContainerRenameReq2) (*types.Resp, error) {
	resp := &types.Resp{}
	name := strings.TrimSpace(req.NewName)
	if name == "" {
		return fail(resp, "新名称不能为空"), nil
	}
	if err := l.ops.Rename(req.Id, name); err != nil {
		return fail(resp, err.Error()), nil
	}
	return ok(resp), nil
}
