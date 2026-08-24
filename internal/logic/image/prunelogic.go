package image

import (
	"context"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type PruneLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPruneLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PruneLogic {
	return &PruneLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Prune 提交异步批量清理镜像任务，立即返回 taskID，前端通过 /api/progress/:taskid 轮询。
func (l *PruneLogic) Prune(req *types.PruneImagesReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if len(req.Ids) == 0 {
		resp.Code = 400
		resp.Msg = "没有需要清理的镜像"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}

	taskID := uuid.New().String()
	ids := req.Ids
	force := req.Force
	hostID := req.HostID
	// resourceID 带上 hostId，避免不同主机的清理任务被误判为同一资源而互斥
	resourceID := "image_prune"
	if hostID != "" {
		resourceID = "image_prune-" + hostID
	}
	// 通过任务管理器提交：按 hostId 路由到对应主机清理（空表示本地）
	startErr := l.svcCtx.TaskManager.TryStart(taskID, resourceID, svc.TaskTypeImagePrune, func(taskCtx context.Context) {
		utiles.PruneImagesOnHost(taskCtx, l.svcCtx, hostID, taskID, ids, force)
	})
	if startErr != nil {
		resp.Code = 400
		resp.Msg = startErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}
