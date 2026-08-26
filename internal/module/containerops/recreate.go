package containerops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// EditSpec 承载容器重建时的可编辑字段。
// nil 切片 / 空字符串 / 零值指针表示"保留原值"，非零值则整体覆盖对应配置。
type EditSpec struct {
	Image         string
	Env           []string
	RestartPolicy string
	PortBindings  []string
	KeepOld       bool
	// Binds 卷/绑定挂载列表（形如 "/host:/container:ro"），非 nil 时整体替换。
	Binds []string
	// NetworkMode 网络模式（bridge/host/none/container:xxx/自定义网络名），空表示不改。
	NetworkMode string
	// Labels 容器标签，非 nil 时整体替换。
	Labels map[string]string
	// Cmd 启动命令（覆盖镜像 CMD），非 nil 时整体替换。
	Cmd []string
	// Entrypoint 入口点（覆盖镜像 ENTRYPOINT），非 nil 时整体替换。
	Entrypoint []string
	// Memory 内存硬限制（字节），指针非 nil 时应用（0 表示不限制）。
	Memory *int64
	// MemorySwap 内存+swap 限制（字节），指针非 nil 时应用。
	MemorySwap *int64
	// NanoCPUs CPU 限额（cpus × 1e9），指针非 nil 时应用（0 表示不限制）。
	NanoCPUs *int64
}

// Recreate 通过"停止旧容器 -> 重命名旧容器 -> 用新配置创建 -> 启动 -> 可选删除旧容器"
// 实现参数编辑。失败时尝试恢复旧容器名并重启，尽量避免服务丢失。
// progress 回调用于向任务系统上报阶段（可为 nil）。
func (s *Service) Recreate(ctx context.Context, id string, spec EditSpec, progress func(pct int, msg string)) error {
	report := func(pct int, msg string) {
		if progress != nil {
			progress(pct, msg)
		}
	}
	// 按 Service 绑定的主机取 client，保证远程主机的容器也能正确重建；
	// 主机不可达时直接报错，绝不回退到本地，避免误操作本地同名容器
	cli, err := s.cliOrErr()
	if err != nil {
		return err
	}

	report(10, "读取原容器配置")
	inspected, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		return fmt.Errorf("获取容器信息失败: %w", err)
	}
	if inspected.Config == nil || inspected.HostConfig == nil {
		return fmt.Errorf("容器配置不可用")
	}
	name := strings.TrimPrefix(inspected.Name, "/")

	// 基于原配置应用变更
	newConfig := *inspected.Config
	if spec.Image != "" {
		newConfig.Image = spec.Image
	}
	if spec.Env != nil {
		newConfig.Env = spec.Env
	}
	// 启动命令 / 入口点：非 nil 时整体覆盖
	if spec.Cmd != nil {
		newConfig.Cmd = spec.Cmd
	}
	if spec.Entrypoint != nil {
		newConfig.Entrypoint = spec.Entrypoint
	}
	// 标签：非 nil 时整体替换
	if spec.Labels != nil {
		newConfig.Labels = spec.Labels
	}
	newConfig.Hostname = ""

	newHostConfig := *inspected.HostConfig
	if spec.RestartPolicy != "" {
		newHostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)}
	}
	if spec.PortBindings != nil {
		bindings, exposed, perr := parsePortBindings(spec.PortBindings)
		if perr != nil {
			return perr
		}
		newHostConfig.PortBindings = bindings
		// ExposedPorts 可能为 nil，需先初始化再写入，避免 panic
		if newConfig.ExposedPorts == nil {
			newConfig.ExposedPorts = nat.PortSet{}
		}
		for p := range exposed {
			newConfig.ExposedPorts[p] = struct{}{}
		}
	}
	// 挂载：非 nil 时整体替换 Binds（形如 "/host:/container:ro"）
	if spec.Binds != nil {
		newHostConfig.Binds = spec.Binds
	}
	// 网络模式：非空时覆盖
	if spec.NetworkMode != "" {
		newHostConfig.NetworkMode = container.NetworkMode(spec.NetworkMode)
	}
	// 资源限制：指针非 nil 时应用（0 表示不限制）
	if spec.Memory != nil {
		newHostConfig.Memory = *spec.Memory
	}
	if spec.MemorySwap != nil {
		newHostConfig.MemorySwap = *spec.MemorySwap
	}
	if spec.NanoCPUs != nil {
		newHostConfig.NanoCPUs = *spec.NanoCPUs
	}

	// 网络端点配置：若改了网络模式，原 EndpointsConfig 可能与新模式冲突，需清空由 Docker 重建
	networkingConfig := &network.NetworkingConfig{EndpointsConfig: inspected.NetworkSettings.Networks}
	if spec.NetworkMode != "" {
		networkingConfig = &network.NetworkingConfig{}
	}

	// 修正非标准守护进程（典型为群晖 DSM）返回的配置，避免删除旧容器后创建失败
	utiles.SanitizeCreateConfig(name, &newConfig, &newHostConfig, networkingConfig)

	report(30, "停止旧容器")
	timeout := 10
	_ = cli.ContainerStop(ctx, id, container.StopOptions{Signal: "SIGINT", Timeout: &timeout})

	report(45, "重命名旧容器")
	backupName := name + "-old-" + time.Now().Format("20060102150405")
	if err := cli.ContainerRename(context.Background(), id, backupName); err != nil {
		return fmt.Errorf("重命名旧容器失败: %w", err)
	}

	report(60, "创建新容器")
	created, err := cli.ContainerCreate(context.Background(), &newConfig, &newHostConfig, networkingConfig, nil, name)
	if err != nil {
		// 回滚：恢复旧容器名并重启
		_ = cli.ContainerRename(context.Background(), id, name)
		_ = cli.ContainerStart(context.Background(), id, container.StartOptions{})
		return fmt.Errorf("创建新容器失败，已回滚: %w", err)
	}

	report(80, "启动新容器")
	if err := cli.ContainerStart(context.Background(), created.ID, container.StartOptions{}); err != nil {
		// 回滚：删除新容器，恢复旧容器
		_ = cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
		_ = cli.ContainerRename(context.Background(), id, name)
		_ = cli.ContainerStart(context.Background(), id, container.StartOptions{})
		return fmt.Errorf("启动新容器失败，已回滚: %w", err)
	}

	if !spec.KeepOld {
		report(95, "删除旧容器")
		_ = cli.ContainerRemove(context.Background(), id, container.RemoveOptions{})
	}

	// 重建成功后清理历史 -old- 备份，避免无限累积：
	// KeepOld=true 本次留了 1 个 → 只保留最新 1 个；KeepOld=false 本次已删 → 清空全部历史残留。
	keep := 0
	if spec.KeepOld {
		keep = 1
	}
	utiles.CleanupOldBackups(cli, name, keep)

	report(100, "参数编辑完成")
	return nil
}

// parsePortBindings 解析 "hostPort:containerPort/proto" 列表为 Docker 端口结构。
func parsePortBindings(specs []string) (nat.PortMap, nat.PortSet, error) {
	bindings := nat.PortMap{}
	exposed := nat.PortSet{}
	for _, s := range specs {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("端口映射格式错误: %s", s)
		}
		hostPort := parts[0]
		containerPart := parts[1]
		if !strings.Contains(containerPart, "/") {
			containerPart += "/tcp"
		}
		port, err := nat.NewPort(strings.SplitN(containerPart, "/", 2)[1], strings.SplitN(containerPart, "/", 2)[0])
		if err != nil {
			return nil, nil, fmt.Errorf("解析端口失败 %s: %w", s, err)
		}
		bindings[port] = append(bindings[port], nat.PortBinding{HostPort: hostPort})
		exposed[port] = struct{}{}
	}
	return bindings, exposed, nil
}
