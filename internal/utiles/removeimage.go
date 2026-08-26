package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// RemoveImage 删除本地主机镜像（保留原签名，兼容既有调用）。
func RemoveImage(ctx *svc.ServiceContext, imageID string, force bool) error {
	return RemoveImageOnHost(ctx, "", imageID, force)
}

// RemoveImageOnHost 在指定 Docker 主机上删除镜像。hostID 为空表示本地。
// 主机不可达时直接报错，绝不回退本地：删除不可逆，回退会误删本地镜像。
func RemoveImageOnHost(ctx *svc.ServiceContext, hostID string, imageID string, force bool) error {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	_, err := cli.ImageRemove(context.Background(), imageID, image.RemoveOptions{Force: force})
	if err != nil {
		return err
	}
	return nil
}
