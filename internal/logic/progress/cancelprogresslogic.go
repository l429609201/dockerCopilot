package progress

import (
	"context"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CancelProgressLogic 处理取消正在执行的任务请求。
type CancelProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelProgressLogic {
	return &CancelProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CancelProgress 通过任务管理器向指定任务发送取消信号。
// 处于关键区（已停止旧容器等）的任务不会被强行中断，仅可取消处于拉取等可中断阶段的任务。
func (l *CancelProgressLogic) CancelProgress(req *types.GetProgressReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if ok := l.svcCtx.TaskManager.Cancel(req.TaskId); !ok {
		resp.Code = 400
		resp.Msg = "任务不存在或已结束"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "已发送取消信号"
	resp.Data = map[string]interface{}{"taskID": req.TaskId}
	return resp, nil
}
