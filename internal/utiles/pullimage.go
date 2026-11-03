package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// PullImageOnHost 在指定主机上拉取镜像，异步解析进度并更新任务中心。
// 仅拉取镜像，不涉及容器的停止、删除或重建。
// registryAuth 为空时等价于匿名拉取。
func PullImageOnHost(ctx context.Context, svcCtx *svc.ServiceContext, hostID string, imageNameAndTag string, registryAuth string, taskID string) error {
	// 按 hostID 路由到对应 Docker 主机（空表示本地）
	cli, err := svcCtx.DockerManager.Get(hostID)
	if err != nil {
		return fmt.Errorf("获取 Docker 客户端失败 [主机%s]: %w", displayHostID(hostID), err)
	}

	// 初始进度
	progress := svc.TaskProgress{
		TaskID:     taskID,
		Message:    fmt.Sprintf("正在拉取镜像 %s", imageNameAndTag),
		Percentage: 5,
		IsDone:     false,
		TaskType:   svc.TaskTypeImagePull,
	}
	svcCtx.UpdateProgress(taskID, progress)

	// 协商 API 版本
	cli.NegotiateAPIVersion(ctx)

	// 携带凭据拉取镜像；registryAuth 为空时等价于匿名拉取
	reader, err := cli.ImagePull(ctx, imageNameAndTag, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		logx.Errorf("拉取镜像失败 [主机%s] [镜像%s]: %v", displayHostID(hostID), imageNameAndTag, err)
		progress.TaskID = taskID
		progress.Message = "拉取镜像失败"
		progress.DetailMsg = err.Error()
		progress.Percentage = 0
		progress.IsDone = true
		progress.Failed = true
		progress.TaskType = svc.TaskTypeImagePull
		svcCtx.UpdateProgress(taskID, progress)
		return fmt.Errorf("拉取镜像失败: %w", err)
	}
	defer reader.Close()

	// 解析拉取进度流（会阻塞直到拉取完成或失败）
	if err := decodePullResp(ctx, reader, svcCtx, taskID); err != nil {
		return err
	}

	// 拉取成功，标记完成
	progress, _ = svcCtx.GetProgress(taskID)
	progress.TaskID = taskID
	progress.Message = fmt.Sprintf("镜像 %s 拉取完成", imageNameAndTag)
	progress.Percentage = 100
	progress.IsDone = true
	progress.TaskType = svc.TaskTypeImagePull
	svcCtx.UpdateProgress(taskID, progress)

	logx.Infof("镜像拉取完成 [主机%s] [镜像%s]", displayHostID(hostID), imageNameAndTag)
	return nil
}
