package utiles

import (
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// StopContainer 停止本地主机上的容器（保留原签名，兼容既有调用）。
func StopContainer(ctx *svc.ServiceContext, id string) error {
	return StopContainerOnHost(ctx, "", id)
}

// StopContainerOnHost 停止指定 Docker 主机上的容器，hostID 为空即本地。
func StopContainerOnHost(ctx *svc.ServiceContext, hostID, id string) error {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		cli = ctx.DockerClient
	}
	timeout := 10
	stopOptions := container.StopOptions{Signal: "SIGINT", Timeout: &timeout}
	return cli.ContainerStop(context.Background(), id, stopOptions)
}
