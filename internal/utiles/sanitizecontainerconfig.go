package utiles

// 本文件用于修正部分非标准 Docker 守护进程（典型为群晖 DSM 的 Container Manager /
// Docker 套件）返回的 inspect 结果与 Docker 官方 API 不一致的问题。
//
// 背景：容器更新流程是「停止旧容器 → 删除旧容器 → 用旧配置创建新容器」。
// 若旧配置本身不被 ContainerCreate 接受，就会出现「旧容器已删除、新容器没建起来」
// 的半更新状态，用户表现为容器凭空消失。群晖环境下这类上报最集中。
//
// 已知不一致点（与 watchtower 上游针对 Synology 的修复一致）：
//  1. Config.ExposedPorts 与 HostConfig.PortBindings 不同步：有端口映射但
//     ExposedPorts 为 nil。官方 Docker 会自动补齐，群晖不会，创建时报错。
//  2. PortBindings 中存在端口号为空的键（形如 "/tcp"）：ContainerCreate 会以
//     "invalid port range: value is empty" 拒绝。
//  3. HostConfig.Devices 的 CgroupPermissions 为空：创建时报 "empty device mode"。
//  4. 非 host 网络下 EndpointsConfig 里残留 MacAddress，与新容器网络配置冲突。
//  5. daemon API < 1.44 不支持 per-network MacAddress，带上会被 SDK 直接拒绝
//     （报错 "specify mac-address per network" requires API version 1.44）。
//  6. NetworkingConfig.EndpointsConfig 包含运行态字段（EndpointID、Gateway、
//     IPAddress、DNSNames 等）：官方 daemon 会自动忽略，但非标准 daemon 可能
//     校验并拒绝启动（报错 "endpoint already exists" 或类似网络冲突错误）。
//
// 处理原则：只做「补齐」与「剔除明显非法值」，不改变用户的有效配置语义。

import (
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/go-connections/nat"
	"github.com/zeromicro/go-zero/core/logx"
)

// endpointMacMinAPIVersion per-network MacAddress 字段要求的最低 daemon API 版本。
// 与 SDK client.ContainerCreate 内部的版本门禁保持一致。
const endpointMacMinAPIVersion = "1.44"

// SanitizeCreateConfig 在创建新容器前修正 inspect 结果中的非标准字段。
//
// 必须在停止/删除旧容器之前调用：这样配置不兼容时可以提前失败，
// 旧容器仍完好，不会出现「删了旧的、新的没起来」的情况。
//
// containerName 仅用于日志定位；apiVersion 为与 daemon 协商后的 API 版本
// （取自 cli.ClientVersion()），空串表示未知、跳过版本相关的清理。
// 函数原地修改传入的配置对象。
func SanitizeCreateConfig(containerName, apiVersion string, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig) {
	if config == nil || hostConfig == nil {
		return
	}

	sanitizePorts(containerName, config, hostConfig)
	sanitizeDevices(containerName, hostConfig)
	sanitizeEndpointMac(containerName, apiVersion, hostConfig, networkingConfig)
}

// sanitizePorts 补齐 ExposedPorts 并剔除端口号为空的非法映射。
func sanitizePorts(containerName string, config *container.Config, hostConfig *container.HostConfig) {
	// 有端口映射但 ExposedPorts 为 nil 时先初始化，否则后续写入会 panic，
	// 且部分守护进程会因缺少 ExposedPorts 直接拒绝创建。
	if len(hostConfig.PortBindings) > 0 && config.ExposedPorts == nil {
		config.ExposedPorts = nat.PortSet{}
		logx.Infof("容器 %s: 检测到有端口映射但 ExposedPorts 为空，已补齐（常见于群晖 DSM）", containerName)
	}

	// 剔除端口号为空的键（形如 "/tcp"），Docker 会以 invalid port range 拒绝创建。
	for port := range hostConfig.PortBindings {
		if port.Port() != "" {
			continue
		}
		delete(hostConfig.PortBindings, port)
		delete(config.ExposedPorts, port)
		logx.Errorf("容器 %s: 剔除非法端口映射 %q（端口号为空，会导致创建失败）", containerName, string(port))
	}

	// 用 PortBindings 反向补齐 ExposedPorts：群晖环境下两者常不同步，
	// 缺失的暴露端口会让新容器丢失端口映射。
	for port := range hostConfig.PortBindings {
		if _, ok := config.ExposedPorts[port]; ok {
			continue
		}
		config.ExposedPorts[port] = struct{}{}
		logx.Infof("容器 %s: 依据端口映射补齐暴露端口 %s", containerName, string(port))
	}
}

// sanitizeDevices 为设备映射补齐默认的 cgroup 权限。
func sanitizeDevices(containerName string, hostConfig *container.HostConfig) {
	// CgroupPermissions 为空时 Docker 报 "empty device mode"。
	// Docker 对未显式声明权限的设备等价于 "rwm"，这里显式补上。
	for i := range hostConfig.Devices {
		if hostConfig.Devices[i].CgroupPermissions != "" {
			continue
		}
		hostConfig.Devices[i].CgroupPermissions = "rwm"
		logx.Infof("容器 %s: 设备 %s 的 cgroup 权限为空，已补为 rwm", containerName, hostConfig.Devices[i].PathOnHost)
	}
}

// sanitizeEndpointMac 清理网络端点配置中的运行态字段与冲突值。
//
// Docker daemon 在容器启动时会自动分配 EndpointID、Gateway、IPAddress 等运行态字段，
// inspect 返回的 EndpointsConfig 直接包含这些字段。官方 daemon 在 ContainerCreate 时
// 会自动忽略它们，但非标准 daemon（群晖、定制环境）可能会校验并拒绝启动。
//
// 本函数清理策略：
//   1. 清除所有运行态字段（EndpointID、Gateway、IPAddress、DNSNames 等）
//   2. 保留用户有效配置（Aliases、Links、DriverOpts、IPAMConfig）
//   3. host 网络模式或低版本 API 下额外清除 MacAddress
func sanitizeEndpointMac(containerName, apiVersion string, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig) {
	if networkingConfig == nil || len(networkingConfig.EndpointsConfig) == 0 {
		return
	}

	// 判定 MAC 地址是否需要清除（满足任一条件即清）
	//
	// 1) host 网络模式：该模式下不允许指定 MAC，带上会被守护进程拒绝。
	//    这里不用 NetworkMode.IsHost()：该方法在 Docker SDK 中是平台相关实现，
	//    在 Windows 编译目标下对 "host" 会返回 false，行为不一致。直接比较字符串。
	isHostNetwork := string(hostConfig.NetworkMode) == "host"

	// 2) daemon API < 1.44：per-network MacAddress 是 1.44 才引入的字段，
	//    低版本 daemon 下 SDK 会在本地直接拒绝请求（不发出），导致
	//    「旧容器已删、新容器建不起来」。此时 MAC 本就无法生效，清掉是安全的。
	//    apiVersion 为空表示未知，保守起见不做处理。
	isLegacyAPI := apiVersion != "" && versions.LessThan(apiVersion, endpointMacMinAPIVersion)
	shouldClearMac := isHostNetwork || isLegacyAPI

	macReason := ""
	if shouldClearMac {
		if isHostNetwork {
			macReason = "host 网络模式"
		} else {
			macReason = "daemon API 版本 " + apiVersion + " 低于 " + endpointMacMinAPIVersion + "，不支持 per-network MAC"
		}
	}

	// 逐网络清理运行态字段
	for netName, endpoint := range networkingConfig.EndpointsConfig {
		if endpoint == nil {
			continue
		}
		hadOperationalData := false

		// 清除运行态字段（daemon 自动分配，不应由客户端提供）
		if endpoint.EndpointID != "" {
			endpoint.EndpointID = ""
			hadOperationalData = true
		}
		if endpoint.Gateway != "" {
			endpoint.Gateway = ""
			hadOperationalData = true
		}
		if endpoint.IPAddress != "" {
			endpoint.IPAddress = ""
			hadOperationalData = true
		}
		if endpoint.GlobalIPv6Address != "" {
			endpoint.GlobalIPv6Address = ""
			hadOperationalData = true
		}
		if endpoint.IPv6Gateway != "" {
			endpoint.IPv6Gateway = ""
			hadOperationalData = true
		}
		if len(endpoint.DNSNames) > 0 {
			endpoint.DNSNames = nil
			hadOperationalData = true
		}

		// 按条件清除 MAC 地址
		if shouldClearMac && endpoint.MacAddress != "" {
			endpoint.MacAddress = ""
			logx.Infof("容器 %s: %s，已清除网络 %s 的 MAC 地址", containerName, macReason, netName)
		}

		if hadOperationalData {
			logx.Infof("容器 %s: 已清除网络 %s 的运行态字段（EndpointID/Gateway/IPAddress/DNSNames 等）", containerName, netName)
		}
	}
}
