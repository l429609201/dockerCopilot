package ops

import (
	"context"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// EditLogic 处理容器参数编辑（任务化重建）。
type EditLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	ops    *containerops.Service
}

func NewEditLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EditLogic {
	return &EditLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		ops:    containerops.New(svcCtx),
	}
}

// Edit 以任务化方式重建容器完成参数编辑，返回 taskID 供前端轮询进度。
// 使用 TaskManager 对同一容器去重，防止并发重建。
func (l *EditLogic) Edit(req *types.ContainerEditReq) (*types.Resp, error) {
	resp := &types.Resp{}
	spec := containerops.EditSpec{
		Image:         req.Image,
		Env:           req.Env,
		RestartPolicy: req.RestartPolicy,
		PortBindings:  req.PortBindings,
		KeepOld:       req.KeepOldContainer,
		Binds:         req.Binds,
		NetworkMode:   req.NetworkMode,
		Labels:        req.Labels,
		Cmd:           req.Cmd,
		Entrypoint:    req.Entrypoint,
		Memory:        req.Memory,
		MemorySwap:    req.MemorySwap,
		NanoCPUs:      req.NanoCPUs,
	}
	taskID := uuid.New().String()
	id := req.Id
	startErr := l.svcCtx.TaskManager.TryStart(taskID, id, svc.TaskTypeContainerUpdate, func(taskCtx context.Context) {
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID: taskID, Name: "编辑容器参数", Percentage: 5,
			Message: "开始重建", DetailMsg: "开始重建", TaskType: svc.TaskTypeContainerUpdate, ResourceID: id,
		})
		err := l.ops.Recreate(taskCtx, id, spec, func(pct int, msg string) {
			l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
				TaskID: taskID, Name: "编辑容器参数", Percentage: pct,
				Message: msg, DetailMsg: msg, TaskType: svc.TaskTypeContainerUpdate, ResourceID: id,
			})
		})
		final := svc.TaskProgress{
			TaskID: taskID, Name: "编辑容器参数", Percentage: 100, IsDone: true,
			TaskType: svc.TaskTypeContainerUpdate, ResourceID: id,
		}
		if err != nil {
			final.Message = "编辑失败"
			final.DetailMsg = err.Error()
			final.Failed = true
			logx.Errorf("容器 %s 参数编辑失败: %v", id, err)
		} else {
			final.Message = "编辑成功"
			final.DetailMsg = "参数已更新并重建"
			logx.Infof("审计：容器 %s 参数已编辑重建", id)
		}
		l.svcCtx.UpdateProgress(taskID, final)
	})
	if startErr != nil {
		return fail(resp, startErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}
