package image

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

type PullLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPullLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PullLogic {
	return &PullLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Pull 拉取镜像：支持指定镜像名、tag、主机和凭据，异步执行并在任务中心展示进度。
func (l *PullLogic) Pull(req *types.PullImageReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}

	if req.ImageName == "" || req.ImageTag == "" {
		resp.Code = 400
		resp.Msg = "镜像名和标签不能为空"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}

	imageNameAndTag := req.ImageName + ":" + req.ImageTag
	taskID := uuid.New().String()

	// 获取 registry 认证（空或"auto"时按镜像名自动匹配；其他值视为明确的凭据ID）
	var registryAuth string
	if req.RegistryID == "" || req.RegistryID == "auto" {
		registryAuth = utiles.GetRegistryAuth(l.svcCtx, imageNameAndTag)
	} else {
		registryAuth = utiles.GetRegistryAuthByID(l.svcCtx, req.RegistryID)
	}

	// 初始任务进度
	l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:     taskID,
		Message:    fmt.Sprintf("开始拉取镜像 %s", imageNameAndTag),
		Percentage: 0,
		IsDone:     false,
		TaskType:   svc.TaskTypeImagePull,
	})

	// 异步拉取镜像
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("拉取镜像 %s panic: %v", imageNameAndTag, r)
				l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
					Message:    fmt.Sprintf("拉取镜像失败: %v", r),
					Percentage: 0,
					IsDone:     true,
				})
			}
		}()

		err := utiles.PullImageOnHost(l.ctx, l.svcCtx, req.HostID, imageNameAndTag, registryAuth, taskID)
		if err != nil {
			logx.Errorf("拉取镜像 %s 失败: %v", imageNameAndTag, err)
			// PullImageOnHost 已经更新了失败状态，这里不需要重复更新
			return
		}
		// PullImageOnHost 已经更新了成功状态
	}()

	resp.Code = 200
	resp.Msg = "镜像拉取任务已创建"
	resp.Data = map[string]interface{}{
		"taskId": taskID,
	}
	return resp, nil
}
