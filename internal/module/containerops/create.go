package containerops

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// CreateSpec 承载「从零创建容器」所需的全部参数。
// 与 EditSpec 不同：这里所有字段都是新容器的完整定义，不存在"保留原值"语义。
type CreateSpec struct {
	Name          string            // 容器名（必填）
	Image         string            // 镜像 name:tag（必填）
	Env           []string          // 环境变量（KEY=VALUE）
	PortBindings  []string          // 端口映射（"8080:80/tcp"）
	Binds         []string          // 卷/绑定挂载（"/host:/container:ro"）
	RestartPolicy string            // 重启策略（no/always/unless-stopped/on-failure）
	NetworkMode   string            // 网络模式（bridge/host/none/自定义网络名）
	Labels        map[string]string // 标签
	Cmd           []string          // 启动命令（覆盖镜像 CMD）
	Entrypoint    []string          // 入口点（覆盖镜像 ENTRYPOINT）
	AutoPull      bool              // 本地无镜像时是否自动拉取
	AutoStart     bool              // 创建后是否立即启动
}

// Create 按 spec 创建一个全新容器（可选自动拉取镜像、自动启动），返回新容器 ID。
// progress 回调用于上报阶段（可为 nil）。
func (s *Service) Create(ctx context.Context, spec CreateSpec, progress func(pct int, msg string)) (string, error) {
	report := func(pct int, msg string) {
		if progress != nil {
			progress(pct, msg)
		}
	}
	cli := s.cli()

	if spec.Name == "" || spec.Image == "" {
		return "", fmt.Errorf("容器名和镜像不能为空")
	}

	// 镜像不存在时按需拉取
	report(10, "检查镜像")
	if _, _, err := cli.ImageInspectWithRaw(ctx, spec.Image); err != nil {
		if !spec.AutoPull {
			return "", fmt.Errorf("本地不存在镜像 %s，请先拉取或勾选自动拉取", spec.Image)
		}
		report(20, "拉取镜像 "+spec.Image)
		rc, perr := cli.ImagePull(ctx, spec.Image, image.PullOptions{})
		if perr != nil {
			return "", fmt.Errorf("拉取镜像失败: %w", perr)
		}
		_, _ = io.Copy(io.Discard, rc) // 读完拉取流，确保镜像落地
		_ = rc.Close()
	}

	// 组装容器配置
	cfg := &container.Config{
		Image:  spec.Image,
		Env:    spec.Env,
		Labels: spec.Labels,
	}
	if len(spec.Cmd) > 0 {
		cfg.Cmd = spec.Cmd
	}
	if len(spec.Entrypoint) > 0 {
		cfg.Entrypoint = spec.Entrypoint
	}

	hostCfg := &container.HostConfig{}
	if spec.RestartPolicy != "" {
		hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)}
	}
	if len(spec.Binds) > 0 {
		hostCfg.Binds = spec.Binds
	}
	if spec.NetworkMode != "" {
		hostCfg.NetworkMode = container.NetworkMode(spec.NetworkMode)
	}
	if len(spec.PortBindings) > 0 {
		bindings, exposed, perr := parsePortBindings(spec.PortBindings)
		if perr != nil {
			return "", perr
		}
		hostCfg.PortBindings = bindings
		cfg.ExposedPorts = nat.PortSet{}
		for p := range exposed {
			cfg.ExposedPorts[p] = struct{}{}
		}
	}

	report(60, "创建容器")
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("创建容器失败: %w", err)
	}

	if spec.AutoStart {
		report(85, "启动容器")
		if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
			// 启动失败不删除容器，保留给用户排查
			return created.ID, fmt.Errorf("容器已创建但启动失败: %w", err)
		}
	}
	report(100, "完成")
	return created.ID, nil
}
