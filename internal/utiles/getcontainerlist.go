package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// GetContainerList 获取本地 Docker 主机的容器列表（保留原签名，兼容既有大量调用）。
func GetContainerList(ctx *svc.ServiceContext) ([]MyType.Container, error) {
	return GetContainerListFromHost(ctx, appconfig.DockerHostLocalID)
}

// GetContainerListFromHost 获取指定 Docker 主机的容器列表，并给每个容器打上主机标记。
// hostID 为空时取本地主机。主机不可达时返回错误，由调用方决定是否忽略。
func GetContainerListFromHost(ctx *svc.ServiceContext, hostID string) ([]MyType.Container, error) {
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		return nil, fmt.Errorf("docker 主机 %s 无可用连接", hostID)
	}
	host, _ := ctx.AppConfig.FindDockerHost(hostID)
	dockerContainerList, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true, // 包含已停止的容器
	})
	if err != nil {
		logx.Errorf("get container list error (host=%s): %v", hostID, err)
		return nil, err
	}
	containerList := make([]MyType.Container, 0, len(dockerContainerList))
	for _, dockerContainerInfo := range dockerContainerList {
		c := MyType.Container{
			Container: dockerContainerInfo,
			HostID:    host.ID,
			HostName:  host.Name,
		}
		// 填充 CreateImage：从 inspect 获取 Config.Image（创建时的镜像名）
		// 这样即使镜像 tag 更新后 Image 字段变空，CreateImage 仍保持稳定
		if inspectData, err := cli.ContainerInspect(context.Background(), dockerContainerInfo.ID); err == nil {
			if inspectData.Config != nil && inspectData.Config.Image != "" {
				c.CreateImage = inspectData.Config.Image
			}
		}
		containerList = append(containerList, c)
	}
	return containerList, nil
}

// GetAllContainers 聚合所有已启用 Docker 主机的容器列表，逐主机标记来源。
// 单个主机连接失败仅记录日志并跳过，不影响其它主机结果。
func GetAllContainers(ctx *svc.ServiceContext) ([]MyType.Container, error) {
	ctx.AppConfig.EnsureLocalHost()
	hosts := ctx.AppConfig.ListDockerHosts()
	var all []MyType.Container
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		list, err := GetContainerListFromHost(ctx, h.ID)
		if err != nil {
			logx.Errorf("聚合容器列表跳过主机[%s:%s]: %v", h.ID, h.Name, err)
			continue
		}
		all = append(all, list...)
	}
	return all, nil
}

func CheckImageUpdate(ctx *svc.ServiceContext, containerListData []MyType.Container) []MyType.Container {
	for i, v := range containerListData {
		// 通过并发安全的方法读取，避免与后台检查 goroutine 并发读写 map 导致进程崩溃
		if needUpdate, ok := ctx.HubImageInfo.NeedUpdate(v.ImageID); ok && needUpdate {
			containerListData[i].Update = true
		}
	}
	return containerListData
}
