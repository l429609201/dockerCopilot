package ops

import (
	"context"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// InspectExecLogic 处理容器详情查看、命令执行与日志读取。
type InspectExecLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	ops    *containerops.Service
}

func NewInspectExecLogic(ctx context.Context, svcCtx *svc.ServiceContext) *InspectExecLogic {
	return &InspectExecLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		ops:    containerops.New(svcCtx),
	}
}

// Inspect 返回容器完整配置（供参数查看/编辑前端使用）。
func (l *InspectExecLogic) Inspect(req *types.ContainerInspectReq) (*types.Resp, error) {
	resp := &types.Resp{}
	info, err := utiles.GetContainerInspect(l.svcCtx, req.Id)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = info
	return resp, nil
}

// Logs 返回容器日志文本。
func (l *InspectExecLogic) Logs(req *types.ContainerLogsReq) (*types.Resp, error) {
	resp := &types.Resp{}
	output, err := l.ops.Logs(l.ctx, req.Id, req.Tail, req.Since, req.Timestamps, 512*1024)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{"logs": output}
	return resp, nil
}

// Exec 在容器内执行命令并返回输出与退出码。
func (l *InspectExecLogic) Exec(req *types.ContainerExecReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if len(req.Cmd) == 0 {
		return fail(resp, "命令不能为空"), nil
	}
	result, err := l.ops.Exec(l.ctx, req.Id, req.Cmd, req.WorkDir, req.User, 60, 64*1024)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	logx.Infof("审计：容器 %s 执行命令 %v，退出码 %d", req.Id, req.Cmd, result.ExitCode)
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"output":   result.Output,
		"exitCode": result.ExitCode,
	}
	return resp, nil
}
