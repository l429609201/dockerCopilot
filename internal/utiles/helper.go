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

// doHelperUpdate 执行实际的停旧→删旧→建新→启动流程（Misaka 方式）。
// 优点：先删除释放名称，再用原名创建，无需重命名，避免重命名失败。
func doHelperUpdate(ctx context.Context, cli *client.Client, targetID, targetName, newImage string, delOld bool) error {
	// 生成固定的时间戳，用于备份名和回滚（统一 {name}-old-{时间戳} 格式，供 CleanupOldBackups 识别）
	timestamp := time.Now().Format("20060102150405")
	backupName := targetName + "-old-" + timestamp

	// backupKept 标记本次是否真的留下了备份容器。
	// 这里 backupName 在函数开头就已无条件生成，不能用它是否为空来判断，故单独用标志位。
	var backupKept bool

	// 清理历史 -old- 备份放在 defer 中执行，成功与失败路径都会收尾。
	// 与 UpdateContainerOnHost 保持一致：keep 依据「本次是否真的留了备份」，
	// 避免失败路径把刚用于回滚的备份误删。
	defer func() {
		keep := 0
		if backupKept {
			keep = 1
		}
		CleanupOldBackups(cli, targetName, keep)
	}()

	// 先 inspect 拿到旧容器完整配置（要在停止/删除前取，配置不受停止影响）
	inspected, err := cli.ContainerInspect(ctx, targetID)
	if err != nil {
		return fmt.Errorf("inspect 主容器失败: %w", err)
	}

	// 准备新容器配置（原配置 + 新镜像）
	cfg := inspected.Config
	cfg.Hostname = "" // 清空，由 Docker 按新容器ID重新生成
	cfg.Image = newImage
	hostCfg := inspected.HostConfig
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: inspected.NetworkSettings.Networks,
	}

	// 修正非标准守护进程（典型为群晖 DSM）返回的配置，避免删除旧容器后创建失败。
	// 与 UpdateContainerOnHost 保持一致：必须放在停止/删除旧容器之前，
	// 保证配置有问题时旧容器仍然完好，可直接返回而无需回滚。
	SanitizeCreateConfig(targetName, cli.ClientVersion(), cfg, hostCfg, netCfg)

	// 停止旧容器（主程序）。给足超时，等它优雅退出。
	timeout := 15
	logx.Info("[helper] 正在停止主容器")
	if err := cli.ContainerStop(ctx, targetID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("停止主容器失败: %w", err)
	}

	// 【Misaka 方式】删除或备份旧容器，释放名称
	if delOld {
		// 直接删除旧容器
		logx.Info("[helper] 正在删除旧容器（释放容器名）")
		if err := cli.ContainerRemove(ctx, targetID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("删除旧容器失败: %w", err)
		}
	} else {
		// 重命名为备份，保留旧容器
		logx.Infof("[helper] 重命名旧容器为 %s（保留备份）", backupName)
		if err := cli.ContainerRename(ctx, targetID, backupName); err != nil {
			return fmt.Errorf("重命名旧容器失败: %w", err)
		}
		backupKept = true
	}

	// 用原配置 + 新镜像 + 原名创建新容器（无需重命名！）
	logx.Infof("[helper] 正在用新镜像创建新容器（使用原名: %s）", targetName)
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, targetName)
	if err != nil {
		// 创建失败：尝试恢复旧容器
		logx.Errorf("[helper] 创建新容器失败，尝试回滚: %v", err)
		if !delOld {
			// 如果旧容器还在（被重命名为备份），尝试改回原名并重启
			_ = cli.ContainerRename(ctx, backupName, targetName)
			_ = cli.ContainerStart(ctx, targetID, container.StartOptions{})
		}
		return fmt.Errorf("创建新容器失败: %w", err)
	}

	// 启动新容器
	logx.Info("[helper] 正在启动新容器")
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		logx.Errorf("[helper] 启动新容器失败，尝试回滚: %v", err)
		_ = cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		if !delOld {
			// 如果旧容器还在（被重命名为备份），尝试改回原名并重启
			_ = cli.ContainerRename(ctx, backupName, targetName)
			_ = cli.ContainerStart(ctx, targetID, container.StartOptions{})
		}
		return fmt.Errorf("启动新容器失败: %w", err)
	}

	// 启动成功后等待 2 秒，检查容器是否仍在运行（排除启动后立即崩溃的情况）
	logx.Info("[helper] 等待 2 秒以验证新容器是否稳定运行")
	time.Sleep(2 * time.Second)
	inspect, err := cli.ContainerInspect(ctx, created.ID)
	if err != nil {
		logx.Errorf("[helper] 无法检查新容器状态: %v（已启动但无法确认运行状态）", err)
	} else if !inspect.State.Running {
		logx.Errorf("[helper] 新容器启动后立即退出（ExitCode=%d），回滚", inspect.State.ExitCode)
		// 尝试获取容器日志（前 50 行），帮助诊断问题
		if logs, logErr := cli.ContainerLogs(ctx, created.ID, container.LogsOptions{
			ShowStdout: true, ShowStderr: true, Tail: "50",
		}); logErr == nil {
			defer logs.Close()
			buf := make([]byte, 4096)
			if n, _ := logs.Read(buf); n > 0 {
				logx.Errorf("[helper] 新容器启动失败日志（前 50 行）:\n%s", string(buf[:n]))
			}
		}
		// 回滚：删除崩溃的新容器，重启旧容器
		_ = cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		if !delOld {
			_ = cli.ContainerRename(ctx, backupName, targetName)
			_ = cli.ContainerStart(ctx, targetID, container.StartOptions{})
			logx.Infof("[helper] 已回滚：旧容器 %s 已重启", targetName)
		}
		return fmt.Errorf("新容器启动后立即退出: ExitCode=%d", inspect.State.ExitCode)
	}

	logx.Infof("[helper] ✅ DC 自我更新成功！新容器 %s 已启动并运行正常", targetName)

	// 成功完成后，主动删除 helper 自己（因 selfupdate.go 已改为 AutoRemove=false）
	// 通过容器名找到自己的 ID
	helperName := targetName + "-selfupdate-helper"
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err == nil {
		for _, c := range containers {
			for _, name := range c.Names {
				// Docker API 返回的容器名带前导斜杠
				if name == "/"+helperName || name == helperName {
					logx.Infof("[helper] 自清理：删除 %s (ID=%s)", helperName, c.ID[:12])
					_ = cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
					return nil
				}
			}
		}
	}
	logx.Infof("[helper] 自清理：未找到自己的容器记录（可能已被外部清理）")

	// 历史 -old- 备份的清理已统一移至函数开头的 defer，成功与失败路径都会执行
	return nil
}
