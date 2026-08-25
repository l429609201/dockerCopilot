package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// PruneImages 异步批量删除本地主机镜像（保留原签名，兼容既有调用）。
func PruneImages(taskCtx context.Context, ctx *svc.ServiceContext, taskID string, imageIDs []string, force bool) {
	PruneImagesOnHost(taskCtx, ctx, "", taskID, imageIDs, force)
}

// PruneImagesOnHost 在指定 Docker 主机上异步批量删除镜像：逐个删除并通过任务系统上报进度。
// hostID 为空表示本地；imageIDs 为待删除的镜像ID列表；force 是否强制删除。
// 支持通过 taskCtx 取消（取消后停止后续删除并标记任务取消）。
func PruneImagesOnHost(taskCtx context.Context, ctx *svc.ServiceContext, hostID string, taskID string, imageIDs []string, force bool) {
	total := len(imageIDs)
	progress := svc.TaskProgress{
		TaskID:     taskID,
		TaskType:   svc.TaskTypeImagePrune,
		Name:       "批量清理镜像",
		Percentage: 0,
		Message:    fmt.Sprintf("准备清理 %d 个镜像", total),
	}
	ctx.UpdateProgress(taskID, progress)

	// 定位目标主机客户端；不可达直接标记任务失败，绝不回退本地：
	// 批量删除不可逆，回退会误删本地镜像。
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		progress.IsDone = true
		progress.Failed = true
		progress.Message = fmt.Sprintf("docker 主机 %s 无可用连接，已终止清理", hostID)
		ctx.UpdateProgress(taskID, progress)
		logx.Errorf("批量清理镜像终止：docker 主机 %s 无可用连接", hostID)
		return
	}

	var success, failed int
	for i, id := range imageIDs {
		// 每次删除前检查取消
		if cerr := taskCtx.Err(); cerr != nil {
			progress.IsDone = true
			progress.Canceled = true
			progress.Failed = true
			progress.Message = fmt.Sprintf("已取消，已清理 %d/%d", success, total)
			ctx.UpdateProgress(taskID, progress)
			return
		}

		_, err := cli.ImageRemove(taskCtx, id, image.RemoveOptions{Force: force})
		if err != nil {
			failed++
			progress.DetailMsg = fmt.Sprintf("删除 %s 失败: %v", shortID(id), err)
			logx.Errorf("批量清理删除镜像 %s 失败: %v", id, err)
		} else {
			success++
			progress.DetailMsg = fmt.Sprintf("已删除 %s", shortID(id))
		}
		progress.Percentage = int(float64(i+1) / float64(total) * 100)
		progress.Message = fmt.Sprintf("清理中 %d/%d（成功 %d，失败 %d）", i+1, total, success, failed)
		ctx.UpdateProgress(taskID, progress)
	}

	progress.IsDone = true
	progress.Percentage = 100
	progress.Failed = failed > 0 && success == 0
	progress.Message = fmt.Sprintf("清理完成：成功 %d，失败 %d，共 %d", success, failed, total)
	ctx.UpdateProgress(taskID, progress)
}

// shortID 截取镜像ID前12位便于展示。
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
