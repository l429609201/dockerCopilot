package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// GetContainerInspect 查看本地主机上指定容器的详情（保留原签名，兼容既有调用）。
func GetContainerInspect(ctx *svc.ServiceContext, id string) (types.ContainerJSON, error) {
	return GetContainerInspectFromHost(ctx, "", id)
}

// GetContainerInspectFromHost 查看指定 Docker 主机上容器的详情。hostID 为空时取本地主机。
func GetContainerInspectFromHost(ctx *svc.ServiceContext, hostID, id string) (types.ContainerJSON, error) {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return types.ContainerJSON{}, fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	inspectedContainer, err := cli.ContainerInspect(context.TODO(), id)
	if err != nil {
		return types.ContainerJSON{}, err
	}
	return inspectedContainer, nil
}
