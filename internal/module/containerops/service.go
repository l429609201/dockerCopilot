package containerops

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

// Service 封装容器生命周期操作，供 HTTP handler、Telegram Bot 等复用。
// 单一职责：只做“对某个容器执行某个动作”，不关心调用来源。
type Service struct {
	svcCtx *svc.ServiceContext
}

// New 创建容器操作服务。
func New(svcCtx *svc.ServiceContext) *Service {
	return &Service{svcCtx: svcCtx}
}

// ResolveIDByName 按容器名查找容器ID，未找到返回错误。
func (s *Service) ResolveIDByName(name string) (string, error) {
	list, err := utiles.GetContainerList(s.svcCtx)
	if err != nil {
		return "", err
	}
	target := strings.TrimPrefix(name, "/")
	for _, c := range list {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == target {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("未找到容器: %s", name)
}

// Start 启动容器。
func (s *Service) Start(id string) error {
	return s.svcCtx.DockerClient.ContainerStart(context.Background(), id, container.StartOptions{})
}

// Stop 停止容器，timeout 为优雅停止秒数。
func (s *Service) Stop(id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	opts := container.StopOptions{Signal: "SIGINT", Timeout: &timeoutSec}
	return s.svcCtx.DockerClient.ContainerStop(context.Background(), id, opts)
}

// Restart 重启容器。
func (s *Service) Restart(id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	opts := container.StopOptions{Signal: "SIGINT", Timeout: &timeoutSec}
	return s.svcCtx.DockerClient.ContainerRestart(context.Background(), id, opts)
}

// Pause 暂停容器。
func (s *Service) Pause(id string) error {
	return s.svcCtx.DockerClient.ContainerPause(context.Background(), id)
}

// Unpause 恢复容器。
func (s *Service) Unpause(id string) error {
	return s.svcCtx.DockerClient.ContainerUnpause(context.Background(), id)
}

// Kill 强制终止容器（默认 SIGKILL）。
func (s *Service) Kill(id string) error {
	return s.svcCtx.DockerClient.ContainerKill(context.Background(), id, "SIGKILL")
}

// Remove 删除容器。force 强制删除运行中容器，removeVolumes 删除匿名卷。
func (s *Service) Remove(id string, force, removeVolumes bool) error {
	return s.svcCtx.DockerClient.ContainerRemove(context.Background(), id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
}

// Rename 重命名容器。
func (s *Service) Rename(id, newName string) error {
	return s.svcCtx.DockerClient.ContainerRename(context.Background(), id, newName)
}
