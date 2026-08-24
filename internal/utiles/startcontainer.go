package utiles

import (
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// StartContainer 启动本地主机上的容器（保留原签名，兼容既有调用）。
func StartContainer(ctx *svc.ServiceContext, id string) error {
	return StartContainerOnHost(ctx, "", id)
}

// StartContainerOnHost 启动指定 Docker 主机上的容器，hostID 为空即本地。
func StartContainerOnHost(ctx *svc.ServiceContext, hostID, id string) error {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		cli = ctx.DockerClient
	}
	return cli.ContainerStart(context.Background(), id, container.StartOptions{})
}
