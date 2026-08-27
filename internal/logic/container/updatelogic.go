package container

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
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
	// 从请求中读取是否删除旧容器的参数（前端传递 "true" 或 "false"）
	delOldContainer := req.DelOldContainer != "false"
	// 整体超时时间来自配置，默认 1800 秒
	timeoutSec := l.svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}
	// 按目标镜像所属 registry 自动匹配已保存的登录凭据，避免手动更新时走匿名拉取
	// （匹配不到或凭据无用户名时返回空串，等价于匿名拉取）。与定时更新路径保持一致。
	registryAuth := utiles.MatchRegistryAuthByImage(l.svcCtx.AppConfig, imageNameAndTag)
	// 通过统一任务管理器提交：限制并发、对同一容器去重，避免重复更新和资源打满
	startErr := l.svcCtx.TaskManager.TryStart(taskID, req.Id, svc.TaskTypeContainerUpdate, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		// 更新目标是本程序自身时，走辅助容器方案，避免"自己停自己"导致的半更新卡死。
		if utiles.IsSelfContainer(l.svcCtx, req.Id) {
			if e := utiles.SelfUpdate(ctxWithTimeout, l.svcCtx, req.Id, req.ContainerName, imageNameAndTag, delOldContainer, taskID, registryAuth); e != nil {
				l.Errorf("Error in SelfUpdate: %v", e)
			}
			return
		}
		if e := utiles.UpdateContainerOnHost(ctxWithTimeout, l.svcCtx, req.HostID, req.Id, req.ContainerName, imageNameAndTag, delOldContainer, taskID, registryAuth); e != nil {
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
