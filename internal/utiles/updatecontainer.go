package utiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerMsgType "github.com/docker/docker/pkg/jsonmessage"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateContainer 兼容旧签名的入口，内部使用后台 context 调用带 context 版本。
func UpdateContainer(serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	return UpdateContainerWithContext(context.Background(), serviceContext, id, name, imageNameAndTag, delOldContainer, taskID)
}

// UpdateContainerWithContext 支持取消与超时的容器更新流程（匿名拉取）。
func UpdateContainerWithContext(ctx context.Context, serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	return UpdateContainerWithAuth(ctx, serviceContext, id, name, imageNameAndTag, delOldContainer, taskID, "")
}

// UpdateContainerWithAuth 支持取消、超时与 Registry 认证的容器更新流程。
// registryAuth 为 base64(JSON) 编码的认证信息，空串表示匿名拉取。
// 在关键步骤前检查 ctx 是否已取消，取消时立即中止并标记任务失败，
// 从而支持前端/机器人主动取消长时间运行的更新任务。
func UpdateContainerWithAuth(ctx context.Context, serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string, registryAuth string) error {
	serviceContext.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:     taskID,
		Percentage: 0,
		Name:       name,
		Message:    "正在连接Docker",
		DetailMsg:  "正在连接Docker",
		IsDone:     false,
		TaskType:   svc.TaskTypeContainerUpdate,
		ResourceID: id,
	})
	var oldTaskProgress, result = serviceContext.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	timeout := 10
	signal := "SIGINT"

	// 更新开始前先检查是否已被取消
	if err := ctx.Err(); err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "任务已取消", err)
		return err
	}

	serviceContext.UpdateProgress(taskID, oldTaskProgress)
	oldTaskProgress.Message = "正在拉取新镜像"
	oldTaskProgress.Percentage = 10
	oldTaskProgress.DetailMsg = "正在拉取新镜像"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)
	serviceContext.DockerClient.NegotiateAPIVersion(ctx)
	// 携带凭据拉取私有镜像；registryAuth 为空时等价于匿名拉取
	reader, err := serviceContext.DockerClient.ImagePull(ctx, imageNameAndTag, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "拉取镜像失败", err)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	err = decodePullResp(ctx, reader, serviceContext, taskID)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "拉取镜像失败", err)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	oldTaskProgress, result = serviceContext.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	oldTaskProgress.Message = "拉取镜像成功"
	oldTaskProgress.DetailMsg = "拉取镜像成功"

	// 【特殊处理】如果是 DC 自我更新，交给辅助容器处理
	// 检测逻辑：对比当前容器ID与要更新的容器ID
	selfID := os.Getenv("HOSTNAME") // Docker 容器内 HOSTNAME 通常是容器ID的短格式
	if selfID != "" && strings.HasPrefix(id, selfID) {
		logx.Info("检测到自我更新，启动辅助容器接管")
		return SelfUpdate(ctx, serviceContext, id, name, imageNameAndTag, delOldContainer, taskID, registryAuth)
	}

	// 【Misaka 方式】先停止删除旧容器，再用原名创建新容器
	// 优点：无需重命名，避免重命名失败；逻辑更简单
	oldTaskProgress.Percentage = 30
	oldTaskProgress.Message = "正在获取容器配置"
	oldTaskProgress.DetailMsg = "正在获取容器配置"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 获取旧容器配置
	inspectedContainer, err := serviceContext.DockerClient.ContainerInspect(context.Background(), id)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "获取容器信息失败", err)
		logx.Error("获取容器信息失败" + err.Error())
		return err
	}

	// 【增强】收集旧镜像信息（SHA256、大小）
	oldImageInfo, _, err := serviceContext.DockerClient.ImageInspectWithRaw(context.Background(), inspectedContainer.Image)
	if err != nil {
		logx.Errorf("获取旧镜像信息失败: %v，继续更新流程", err)
	} else {
		// 提取 SHA256（去掉 sha256: 前缀）
		oldDigest := strings.TrimPrefix(oldImageInfo.ID, "sha256:")
		oldTaskProgress.OldImageDigest = oldDigest
		oldTaskProgress.OldImageSize = oldImageInfo.Size
		logx.Infof("旧镜像信息 - Digest: %s, Size: %d bytes", oldDigest, oldImageInfo.Size)
	}

	// 【增强】解析镜像名称和标签
	imageNameParts := strings.Split(imageNameAndTag, ":")
	if len(imageNameParts) == 2 {
		oldTaskProgress.ImageName = imageNameParts[0]
		oldTaskProgress.ImageTag = imageNameParts[1]
	} else {
		oldTaskProgress.ImageName = imageNameAndTag
		oldTaskProgress.ImageTag = "latest"
	}

	// 准备新容器配置（使用新镜像）
	inspectedContainer.Config.Hostname = ""
	inspectedContainer.Config.Image = imageNameAndTag
	inspectedContainer.Image = imageNameAndTag
	config := inspectedContainer.Config
	hostConfig := inspectedContainer.HostConfig
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: inspectedContainer.NetworkSettings.Networks,
	}

	oldTaskProgress.Percentage = 40
	oldTaskProgress.Message = "正在停止旧容器"
	oldTaskProgress.DetailMsg = "正在停止旧容器"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 从停止旧容器开始进入关键区：此后不再响应取消/超时
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	err = serviceContext.DockerClient.ContainerStop(context.Background(), id, stopOptions)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "停止旧容器失败", err)
		return err
	}

	oldTaskProgress.Percentage = 50
	oldTaskProgress.Message = "正在删除旧容器"
	oldTaskProgress.DetailMsg = "正在删除旧容器（释放容器名）"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 删除旧容器（释放容器名，如果配置要求保留则重命名）
	if delOldContainer {
		err = serviceContext.DockerClient.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true})
		if err != nil {
			markTaskFailed(serviceContext, taskID, &oldTaskProgress, "删除旧容器失败", err)
			return err
		}
	} else {
		// 保留旧容器：重命名为带时间戳的备份名
		currentDate := time.Now().Format("2006-01-02-15-04-05")
		backupName := name + "-" + currentDate
		err = serviceContext.DockerClient.ContainerRename(context.Background(), id, backupName)
		if err != nil {
			markTaskFailed(serviceContext, taskID, &oldTaskProgress, "重命名旧容器失败", err)
			return err
		}
		logx.Infof("旧容器已重命名为: %s", backupName)
	}

	oldTaskProgress.Percentage = 70
	oldTaskProgress.Message = "正在创建新容器"
	oldTaskProgress.DetailMsg = fmt.Sprintf("使用原名: %s", name)
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 用原名创建新容器（旧容器已删除或重命名，名称可用）
	newContainerResp, err := serviceContext.DockerClient.ContainerCreate(context.Background(), config, hostConfig, networkingConfig, nil, name)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "创建新容器失败", err)
		return err
	}
	newContainerID := newContainerResp.ID

	oldTaskProgress.Percentage = 90
	oldTaskProgress.Message = "正在启动新容器"
	oldTaskProgress.DetailMsg = "正在启动新容器"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 启动新容器
	err = serviceContext.DockerClient.ContainerStart(context.Background(), newContainerID, container.StartOptions{})
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "启动新容器失败", err)
		return err
	}

	// 【增强】收集新镜像信息（SHA256、大小）
	newImageInfo, _, err := serviceContext.DockerClient.ImageInspectWithRaw(context.Background(), imageNameAndTag)
	if err != nil {
		logx.Errorf("获取新镜像信息失败: %v，但容器已成功启动", err)
	} else {
		// 提取 SHA256（去掉 sha256: 前缀）
		newDigest := strings.TrimPrefix(newImageInfo.ID, "sha256:")
		oldTaskProgress.NewImageDigest = newDigest
		oldTaskProgress.NewImageSize = newImageInfo.Size
		logx.Infof("新镜像信息 - Digest: %s, Size: %d bytes", newDigest, newImageInfo.Size)
	}

	oldTaskProgress.Message = "更新成功"
	oldTaskProgress.DetailMsg = "更新成功"
	oldTaskProgress.Percentage = 100
	oldTaskProgress.IsDone = true
	serviceContext.UpdateProgress(taskID, oldTaskProgress)
	return nil
}

// markTaskFailed 统一将任务标记为失败结束，避免各处重复样板代码。
func markTaskFailed(svcCtx *svc.ServiceContext, taskID string, progress *svc.TaskProgress, message string, cause error) {
	progress.Message = message
	if cause != nil {
		progress.DetailMsg = cause.Error()
	} else {
		progress.DetailMsg = message
	}
	progress.IsDone = true
	progress.Failed = true
	if cause != nil && errors.Is(cause, context.Canceled) {
		progress.Canceled = true
	}
	svcCtx.UpdateProgress(taskID, *progress)
}

func decodePullResp(taskCtx context.Context, reader io.Reader, svcCtx *svc.ServiceContext, taskID string) (err error) {
	decoder := json.NewDecoder(reader)
	var oldTaskProgress, result = svcCtx.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	for {
		// 每次读取前检查是否被取消，实现拉取阶段的可中断
		if cerr := taskCtx.Err(); cerr != nil {
			oldTaskProgress.Message = "拉取镜像已取消"
			oldTaskProgress.DetailMsg = cerr.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			oldTaskProgress.Canceled = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			return fmt.Errorf("拉取镜像已取消: %w", cerr)
		}
		var msg dockerMsgType.JSONMessage
		if err = decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = err.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Failed to decode pull image response: %s", err)
			return fmt.Errorf("拉取镜像失败: %w", err)
		}
		// Print the progress or error information from the response
		if msg.Error != nil {
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = msg.Error.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Error: %s", msg.Error)
			return fmt.Errorf("拉取镜像失败: %w", msg.Error)
		} else {
			var formattedMsg string
			if msg.Progress != nil {
				formattedMsg = fmt.Sprintf("进度%s: %s", msg.Status, msg.Progress.String())
			} else {
				formattedMsg = fmt.Sprintf("进度%s", msg.Status)
			}
			oldTaskProgress.DetailMsg = formattedMsg
			oldTaskProgress.Percentage = 25
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Infof("拉取镜像进度\t %s: %s\n", msg.Status, msg.Progress)
		}
	}
}
