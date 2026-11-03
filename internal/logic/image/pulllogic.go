package image

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
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

	imageName := strings.TrimSpace(req.ImageName)
	imageTag := strings.TrimSpace(req.ImageTag)
	// 悬空镜像（<none>）没有可拉取的引用，直接拒绝
	if imageName == "" || imageTag == "" || imageName == "<none>" || imageTag == "<none>" {
		resp.Code = 400
		resp.Msg = "镜像名和标签不能为空，悬空镜像无法拉取"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}

	imageNameAndTag := imageName + ":" + imageTag
	hostID := req.HostID
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	// 先确认主机可用，避免下发一个必然失败的任务
	if _, ok := l.svcCtx.DockerManager.GetClient(hostID); !ok {
		resp.Code = 400
		resp.Msg = fmt.Sprintf("Docker 主机 %s 不可用", hostID)
		resp.Data = map[string]interface{}{}
		return resp, nil
	}

	// 凭据：空或 "auto" 按镜像所属 registry 自动匹配；其他值按凭据ID精确选择
	registryAuth := ""
	if req.RegistryID == "" || req.RegistryID == utiles.RegistryIDAuto {
		registryAuth = utiles.MatchRegistryAuthByImage(l.svcCtx.AppConfig, imageNameAndTag)
	} else {
		cred, ok := l.svcCtx.AppConfig.FindRegistry(req.RegistryID)
		if !ok {
			resp.Code = 400
			resp.Msg = "指定的仓库凭据不存在"
			resp.Data = map[string]interface{}{}
			return resp, nil
		}
		auth, e := utiles.EncodeRegistryAuth(&cred)
		if e != nil {
			resp.Code = 500
			resp.Msg = "编码仓库凭据失败"
			resp.Data = map[string]interface{}{}
			return resp, nil
		}
		registryAuth = auth
	}

	// 整体超时与容器更新共用配置，默认 1800 秒
	timeoutSec := l.svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	taskID := uuid.New().String()
	// 初始进度，让任务中心立即可见
	l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:   taskID,
		Message:  fmt.Sprintf("等待拉取镜像 %s", imageNameAndTag),
		TaskType: svc.TaskTypeImagePull,
	})

	// 通过统一任务管理器执行：限制并发，同一主机同一镜像不重复拉取。
	// 注意不能用请求 ctx：HTTP 响应返回后请求 ctx 即被取消，会中断后台拉取。
	resourceID := "image_pull|" + hostID + "|" + imageNameAndTag
	startErr := l.svcCtx.TaskManager.TryStart(taskID, resourceID, svc.TaskTypeImagePull, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		if e := utiles.PullImageOnHost(ctxWithTimeout, l.svcCtx, hostID, imageNameAndTag, registryAuth, taskID); e != nil {
			logx.Errorf("拉取镜像 %s 失败: %v", imageNameAndTag, e)
		}
	})
	if startErr != nil {
		// 登记失败（如同一镜像已在拉取）：把预置的进度标记为失败，避免任务中心残留"等待中"
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID:    taskID,
			Message:   "拉取任务未启动",
			DetailMsg: startErr.Error(),
			IsDone:    true,
			Failed:    true,
			TaskType:  svc.TaskTypeImagePull,
		})
		resp.Code = 400
		resp.Msg = startErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}

	resp.Code = 200
	resp.Msg = "镜像拉取任务已创建"
	resp.Data = map[string]interface{}{"taskId": taskID}
	return resp, nil
}
