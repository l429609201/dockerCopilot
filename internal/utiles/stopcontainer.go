package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// StopContainer 停止本地主机上的容器（保留原签名，兼容既有调用）。
func StopContainer(ctx *svc.ServiceContext, id string) error {
	return StopContainerOnHost(ctx, "", id)
}

// StopContainerOnHost 停止指定 Docker 主机上的容器，hostID 为空即本地。
// 主机不可达时直接报错，绝不回退本地：远程与本地容器 ID 可能重合，回退会停错机器上的容器。
func StopContainerOnHost(ctx *svc.ServiceContext, hostID, id string) error {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	timeout := 10
	stopOptions := container.StopOptions{Signal: "SIGINT", Timeout: &timeout}
	return cli.ContainerStop(context.Background(), id, stopOptions)
}
