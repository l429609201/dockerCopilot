package utiles

import (
	"context"
	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// RemoveImage 删除本地主机镜像（保留原签名，兼容既有调用）。
func RemoveImage(ctx *svc.ServiceContext, imageID string, force bool) error {
	return RemoveImageOnHost(ctx, "", imageID, force)
}

// RemoveImageOnHost 在指定 Docker 主机上删除镜像。hostID 为空表示本地。
func RemoveImageOnHost(ctx *svc.ServiceContext, hostID string, imageID string, force bool) error {
	// 定位目标主机客户端；不可用则回退本地，避免 nil 崩溃
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		cli = ctx.DockerClient
	}
	_, err := cli.ImageRemove(context.Background(), imageID, image.RemoveOptions{Force: force})
	if err != nil {
		return err
	}
	return nil
}
