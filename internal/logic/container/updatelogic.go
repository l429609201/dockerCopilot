package container

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
)

type UpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateLogic {
	return &UpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateLogic) Update(req *types.ContainerUpdateReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	taskID := uuid.New().String()
	imageNameAndTag := req.ImageNameAndTag
	delOldContainer := os.Getenv("DelOldContainer") != "false"
	// 整体超时时间来自配置，默认 1800 秒
	timeoutSec := l.svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}
	// 通过统一任务管理器提交：限制并发、对同一容器去重，避免重复更新和资源打满
	startErr := l.svcCtx.TaskManager.TryStart(taskID, req.Id, svc.TaskTypeContainerUpdate, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		// 更新目标是本程序自身时，走辅助容器方案，避免"自己停自己"导致的半更新卡死。
		if utiles.IsSelfContainer(l.svcCtx, req.Id) {
			if e := utiles.SelfUpdate(ctxWithTimeout, l.svcCtx, req.Id, req.ContainerName, imageNameAndTag, delOldContainer, taskID, ""); e != nil {
				l.Errorf("Error in SelfUpdate: %v", e)
			}
			return
		}
		if e := utiles.UpdateContainerWithContext(ctxWithTimeout, l.svcCtx, req.Id, req.ContainerName, imageNameAndTag, delOldContainer, taskID); e != nil {
			l.Errorf("Error in UpdateContainer: %v", e)
		}
	})
	if startErr != nil {
		// 资源重复等登记失败：返回业务错误，不下发无效 taskID
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
