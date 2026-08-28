package utiles

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// TestSanitizeCreateConfig_集成测试_API143环境完整流程 模拟 API 1.43 环境下的完整更新流程。
// 验证所有可能导致创建失败的配置项都被正确清理。
func TestSanitizeCreateConfig_集成测试_API143环境完整流程(t *testing.T) {
	// 模拟从 inspect 获取的旧容器配置（包含所有潜在问题）
	config := &container.Config{
		Image: "nginx:latest",
		// 问题1: ExposedPorts 为 nil（群晖常见）
		ExposedPorts: nil,
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "bridge", // 非 host 网络模式
		// 问题2: 端口映射存在，但有空端口号
		PortBindings: nat.PortMap{
			"80/tcp": {{HostPort: "8080"}},
			"/tcp":   {{HostPort: "9090"}}, // 空端口号，会导致创建失败
		},
		Resources: container.Resources{
			// 问题3: 设备权限为空
			Devices: []container.DeviceMapping{
				{
					PathOnHost:        "/dev/dri",
					PathInContainer:   "/dev/dri",
					CgroupPermissions: "", // 空权限，会导致创建失败
				},
			},
		},
	}

	// 问题4: bridge 网络下有 MAC 地址（API 1.43 不支持）
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"bridge": {
				MacAddress: "02:42:ac:11:00:02", // API 1.43 不支持，会导致创建失败
			},
		},
	}

	// 执行修正（模拟 API 1.43 环境）
	SanitizeCreateConfig("test-container", "1.43", config, hostConfig, networkingConfig)

	// 验证1: ExposedPorts 已补齐
	if config.ExposedPorts == nil {
		t.Fatal("ExposedPorts 应被初始化")
	}
	if _, ok := config.ExposedPorts["80/tcp"]; !ok {
		t.Errorf("应依据 PortBindings 补齐 80/tcp")
	}

	// 验证2: 空端口号映射已剔除
	if _, ok := hostConfig.PortBindings["/tcp"]; ok {
		t.Error("空端口号映射 '/tcp' 应被剔除")
	}
	if _, ok := hostConfig.PortBindings["80/tcp"]; !ok {
		t.Error("有效端口映射 '80/tcp' 应保留")
	}

	// 验证3: 设备权限已补齐
	if hostConfig.Resources.Devices[0].CgroupPermissions != "rwm" {
		t.Errorf("设备权限应补为 'rwm'，实际: %s", hostConfig.Resources.Devices[0].CgroupPermissions)
	}

	// 验证4: API 1.43 环境下 MAC 地址已清除
	if networkingConfig.EndpointsConfig["bridge"].MacAddress != "" {
		t.Errorf("API 1.43 环境下 bridge 网络的 MAC 应被清除，实际: %s",
			networkingConfig.EndpointsConfig["bridge"].MacAddress)
	}

	t.Logf("✅ API 1.43 环境集成测试通过：所有配置问题已修正")
}

// TestSanitizeCreateConfig_集成测试_API144环境完整流程 模拟 API 1.44 环境下的完整更新流程。
// 验证在新版 API 下，per-network MAC 地址被正确保留。
func TestSanitizeCreateConfig_集成测试_API144环境完整流程(t *testing.T) {
	// 模拟从 inspect 获取的旧容器配置
	config := &container.Config{
		Image:        "nginx:latest",
		ExposedPorts: nil,
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "bridge",
		PortBindings: nat.PortMap{
			"80/tcp": {{HostPort: "8080"}},
			"/tcp":   {{HostPort: "9090"}},
		},
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{
					PathOnHost:        "/dev/dri",
					PathInContainer:   "/dev/dri",
					CgroupPermissions: "",
				},
			},
		},
	}

	// bridge 网络下有 MAC 地址（API 1.44 支持）
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"bridge": {
				MacAddress: "02:42:ac:11:00:02", // API 1.44 支持，应保留
			},
		},
	}

	// 执行修正（模拟 API 1.44 环境）
	SanitizeCreateConfig("test-container", "1.44", config, hostConfig, networkingConfig)

	// 验证1-3: 基础配置问题仍然修正
	if config.ExposedPorts == nil {
		t.Fatal("ExposedPorts 应被初始化")
	}
	if _, ok := hostConfig.PortBindings["/tcp"]; ok {
		t.Error("空端口号映射 '/tcp' 应被剔除")
	}
	if hostConfig.Resources.Devices[0].CgroupPermissions != "rwm" {
		t.Errorf("设备权限应补为 'rwm'")
	}

	// 验证4: API 1.44 环境下 MAC 地址应保留
	if networkingConfig.EndpointsConfig["bridge"].MacAddress != "02:42:ac:11:00:02" {
		t.Errorf("API 1.44 环境下 bridge 网络的 MAC 应保留，实际: %s",
			networkingConfig.EndpointsConfig["bridge"].MacAddress)
	}

	t.Logf("✅ API 1.44 环境集成测试通过：MAC 地址正确保留，其他问题已修正")
}
