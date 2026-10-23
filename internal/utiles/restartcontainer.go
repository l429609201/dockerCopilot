package utiles

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

const containerRestartTimeout = 20 * time.Second

// RestartContainer 重启本地主机上的容器（保留原签名，兼容既有调用）。
func RestartContainer(ctx *svc.ServiceContext, id string) error {
	return RestartContainerOnHost(ctx, "", id)
}

// RestartContainerOnHost 重启指定 Docker 主机上的容器，hostID 为空即本地。
// 主机不可达时直接报错，绝不回退本地；请求超时避免调度永久阻塞。
func RestartContainerOnHost(ctx *svc.ServiceContext, hostID, id string) error {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	requestCtx, cancel := context.WithTimeout(context.Background(), containerRestartTimeout)
	defer cancel()
	timeout := int(containerRestartTimeout / time.Second)
	stopOptions := container.StopOptions{Signal: "SIGINT", Timeout: &timeout}
	return cli.ContainerRestart(requestCtx, id, stopOptions)
}
