package containerops

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// Service 封装容器生命周期操作，供 HTTP handler、Telegram Bot 等复用。
// 单一职责：只做“对某个容器执行某个动作”，不关心调用来源。
// hostID 标记操作目标所在的 Docker 主机（多 Docker 管理），空表示本地。
type Service struct {
	svcCtx *svc.ServiceContext
	hostID string
}

// New 创建针对本地 Docker 主机的容器操作服务（保留原签名，兼容既有调用）。
func New(svcCtx *svc.ServiceContext) *Service {
	return &Service{svcCtx: svcCtx}
}

// NewForHost 创建针对指定 Docker 主机的容器操作服务。hostID 为空时等同本地。
func NewForHost(svcCtx *svc.ServiceContext, hostID string) *Service {
	return &Service{svcCtx: svcCtx, hostID: hostID}
}

// cli 返回当前主机对应的 docker client，未找到时回退本地。
func (s *Service) cli() *dockerclient.Client {
	if c, ok := s.svcCtx.DockerManager.GetClient(s.hostID); ok {
		return c
	}
	return s.svcCtx.DockerClient
}

// ResolveIDByName 按容器名查找容器ID，未找到返回错误。
func (s *Service) ResolveIDByName(name string) (string, error) {
	list, err := utiles.GetContainerListFromHost(s.svcCtx, s.hostID)
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
	return s.cli().ContainerStart(context.Background(), id, container.StartOptions{})
}

// Stop 停止容器，timeout 为优雅停止秒数。
func (s *Service) Stop(id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	opts := container.StopOptions{Signal: "SIGINT", Timeout: &timeoutSec}
	return s.cli().ContainerStop(context.Background(), id, opts)
}

// Restart 重启容器。
func (s *Service) Restart(id string, timeoutSec int) error {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	opts := container.StopOptions{Signal: "SIGINT", Timeout: &timeoutSec}
	return s.cli().ContainerRestart(context.Background(), id, opts)
}

// Pause 暂停容器。
func (s *Service) Pause(id string) error {
	return s.cli().ContainerPause(context.Background(), id)
}

// Unpause 恢复容器。
func (s *Service) Unpause(id string) error {
	return s.cli().ContainerUnpause(context.Background(), id)
}

// Kill 强制终止容器（默认 SIGKILL）。
func (s *Service) Kill(id string) error {
	return s.cli().ContainerKill(context.Background(), id, "SIGKILL")
}

// Remove 删除容器。force 强制删除运行中容器，removeVolumes 删除匿名卷。
func (s *Service) Remove(id string, force, removeVolumes bool) error {
	return s.cli().ContainerRemove(context.Background(), id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
}

// Rename 重命名容器。
func (s *Service) Rename(id, newName string) error {
	return s.cli().ContainerRename(context.Background(), id, newName)
}

// Update 更新容器：拉取 imageNameAndTag 指定镜像并重建容器。
// imageNameAndTag 为空时表示沿用容器当前镜像（等同"检查更新后重建"）。
// 复用与 HTTP 层一致的任务管理器与自更新逻辑，返回提交的 taskID。
func (s *Service) Update(id, name, imageNameAndTag string) (string, error) {
	taskID := uuid.New().String()
	delOldContainer := os.Getenv("DelOldContainer") != "false"
	timeoutSec := s.svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}
	// 按目标镜像所属 registry 自动匹配已保存的登录凭据，避免 Bot 手动更新时走匿名拉取
	// （匹配不到或凭据无用户名时返回空串，等价于匿名拉取）。与定时/HTTP 更新路径保持一致。
	registryAuth := utiles.MatchRegistryAuthByImage(s.svcCtx.AppConfig, imageNameAndTag)
	startErr := s.svcCtx.TaskManager.TryStart(taskID, id, svc.TaskTypeContainerUpdate, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		// 目标是本程序自身时，走辅助容器方案，避免"自己停自己"卡死。
		if utiles.IsSelfContainer(s.svcCtx, id) {
			if e := utiles.SelfUpdate(ctxWithTimeout, s.svcCtx, id, name, imageNameAndTag, delOldContainer, taskID, registryAuth); e != nil {
				logx.Errorf("Bot 自更新失败: %v", e)
			}
			return
		}
		if e := utiles.UpdateContainerOnHost(ctxWithTimeout, s.svcCtx, s.hostID, id, name, imageNameAndTag, delOldContainer, taskID, registryAuth); e != nil {
			logx.Errorf("Bot 更新容器失败: %v", e)
		}
	})
	if startErr != nil {
		return "", startErr
	}
	return taskID, nil
}
