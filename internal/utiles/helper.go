package utiles

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/zeromicro/go-zero/core/logx"
)

// IsHelperMode 判断当前进程是否以"自更新辅助容器"模式启动。
func IsHelperMode() bool {
	return os.Getenv(EnvHelperMode) == "1"
}

// RunHelper 以辅助容器模式运行：接管主容器的更新收尾工作。
// 流程：等待主容器可停 → 停旧 → 改名 → inspect → 用原配置+新镜像建新 → 启动 → 可选删旧。
// 辅助容器本身设置了 AutoRemove，退出后会被 Docker 自动清理。
func RunHelper() {
	targetID := os.Getenv(EnvHelperTargetID)
	targetName := os.Getenv(EnvHelperTargetNam)
	newImage := os.Getenv(EnvHelperImage)
	delOld := os.Getenv(EnvHelperDelOld) == "1"

	logx.Infof("[helper] 自更新辅助容器启动: target=%s name=%s image=%s delOld=%v",
		targetID, targetName, newImage, delOld)

	if targetID == "" || targetName == "" || newImage == "" {
		logx.Error("[helper] 缺少必要环境变量，辅助容器退出")
		os.Exit(1)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logx.Errorf("[helper] 创建 Docker 客户端失败: %v", err)
		os.Exit(1)
	}
	ctx := context.Background()

	if err := doHelperUpdate(ctx, cli, targetID, targetName, newImage, delOld); err != nil {
		logx.Errorf("[helper] 自更新失败: %v", err)
		os.Exit(1)
	}
	logx.Info("[helper] 自更新完成，辅助容器退出")
	os.Exit(0)
}

// doHelperUpdate 执行实际的停旧→建新→启动→删旧流程。
func doHelperUpdate(ctx context.Context, cli *client.Client, targetID, targetName, newImage string, delOld bool) error {
	// 先 inspect 拿到旧容器完整配置（要在停止/删除前取，配置不受停止影响）
	inspected, err := cli.ContainerInspect(ctx, targetID)
	if err != nil {
		return fmt.Errorf("inspect 主容器失败: %w", err)
	}

	// 停止旧容器（主程序）。给足超时，等它优雅退出。
	timeout := 15
	logx.Info("[helper] 正在停止主容器")
	if err := cli.ContainerStop(ctx, targetID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("停止主容器失败: %w", err)
	}

	// 重命名旧容器，腾出原名给新容器
	backupName := targetName + "-old-" + time.Now().Format("20060102-150405")
	logx.Infof("[helper] 重命名旧容器为 %s", backupName)
	if err := cli.ContainerRename(ctx, targetID, backupName); err != nil {
		return fmt.Errorf("重命名旧容器失败: %w", err)
	}

	// 用原配置 + 新镜像创建新容器
	cfg := inspected.Config
	cfg.Hostname = "" // 清空，由 Docker 按新容器ID重新生成
	cfg.Image = newImage
	hostCfg := inspected.HostConfig
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: inspected.NetworkSettings.Networks,
	}
	logx.Info("[helper] 正在用新镜像创建新容器")
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, targetName)
	if err != nil {
		// 创建失败：尝试把旧容器改回原名并重启，尽量回滚
		logx.Errorf("[helper] 创建新容器失败，尝试回滚: %v", err)
		_ = cli.ContainerRename(ctx, targetID, targetName)
		_ = cli.ContainerStart(ctx, targetID, container.StartOptions{})
		return fmt.Errorf("创建新容器失败: %w", err)
	}

	// 启动新容器
	logx.Info("[helper] 正在启动新容器")
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		logx.Errorf("[helper] 启动新容器失败，尝试回滚: %v", err)
		_ = cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		_ = cli.ContainerRename(ctx, targetID, targetName)
		_ = cli.ContainerStart(ctx, targetID, container.StartOptions{})
		return fmt.Errorf("启动新容器失败: %w", err)
	}

	// 可选删除旧容器
	if delOld {
		logx.Info("[helper] 删除旧容器")
		if err := cli.ContainerRemove(ctx, targetID, container.RemoveOptions{Force: true}); err != nil {
			// 删除失败不算致命：新容器已起来
			logx.Errorf("[helper] 删除旧容器失败（不影响更新结果）: %v", err)
		}
	}
	return nil
}
