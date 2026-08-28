package utiles

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// 群晖 DSM 典型现象：有端口映射但 ExposedPorts 为 nil，创建新容器时被拒绝。
func TestSanitizeCreateConfig_补齐缺失的暴露端口(t *testing.T) {
	config := &container.Config{ExposedPorts: nil}
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"80/tcp": []nat.PortBinding{{HostPort: "8080"}},
		},
	}

	SanitizeCreateConfig("test", "1.44", config, hostConfig, nil)

	if config.ExposedPorts == nil {
		t.Fatal("ExposedPorts 应被初始化")
	}
	if _, ok := config.ExposedPorts["80/tcp"]; !ok {
		t.Errorf("应依据 PortBindings 补齐 80/tcp，实际: %v", config.ExposedPorts)
	}
}

// 端口号为空的映射会让 ContainerCreate 报 invalid port range，必须剔除。
func TestSanitizeCreateConfig_剔除空端口映射(t *testing.T) {
	config := &container.Config{
		ExposedPorts: nat.PortSet{"/tcp": struct{}{}, "80/tcp": struct{}{}},
	}
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"/tcp":   []nat.PortBinding{{HostPort: "8080"}},
			"80/tcp": []nat.PortBinding{{HostPort: "8081"}},
		},
	}

	SanitizeCreateConfig("test", "1.44", config, hostConfig, nil)

	if _, ok := hostConfig.PortBindings["/tcp"]; ok {
		t.Error("空端口号的映射应被剔除")
	}
	if _, ok := config.ExposedPorts["/tcp"]; ok {
		t.Error("空端口号的暴露端口应被剔除")
	}
	if _, ok := hostConfig.PortBindings["80/tcp"]; !ok {
		t.Error("正常端口映射不应被误删")
	}
}

// CgroupPermissions 为空时 Docker 报 empty device mode。
func TestSanitizeCreateConfig_补齐设备权限(t *testing.T) {
	config := &container.Config{}
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{PathOnHost: "/dev/dri", CgroupPermissions: ""},
				{PathOnHost: "/dev/net/tun", CgroupPermissions: "rw"},
			},
		},
	}

	SanitizeCreateConfig("test", "1.44", config, hostConfig, nil)

	if hostConfig.Devices[0].CgroupPermissions != "rwm" {
		t.Errorf("空权限应补为 rwm，实际: %q", hostConfig.Devices[0].CgroupPermissions)
	}
	if hostConfig.Devices[1].CgroupPermissions != "rw" {
		t.Errorf("已有权限不应被覆盖，实际: %q", hostConfig.Devices[1].CgroupPermissions)
	}
}

// host 网络模式下携带 MAC 地址会被守护进程拒绝。
func TestSanitizeCreateConfig_清理host模式下的MAC残留(t *testing.T) {
	config := &container.Config{}
	hostConfig := &container.HostConfig{NetworkMode: "host"}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"host": {MacAddress: "02:42:ac:11:00:02"},
		},
	}

	SanitizeCreateConfig("test", "1.44", config, hostConfig, networkingConfig)

	if got := networkingConfig.EndpointsConfig["host"].MacAddress; got != "" {
		t.Errorf("host 模式下 MAC 应被清空，实际: %q", got)
	}
}

// 非 host 网络模式下 MAC 地址是有效配置，不能动。
func TestSanitizeCreateConfig_保留bridge模式下的MAC(t *testing.T) {
	config := &container.Config{}
	hostConfig := &container.HostConfig{NetworkMode: "bridge"}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"bridge": {MacAddress: "02:42:ac:11:00:02"},
		},
	}

	SanitizeCreateConfig("test", "1.44", config, hostConfig, networkingConfig)

	if got := networkingConfig.EndpointsConfig["bridge"].MacAddress; got != "02:42:ac:11:00:02" {
		t.Errorf("bridge 模式下 MAC 应保留，实际: %q", got)
	}
}

// daemon API 低于 1.44 时不支持 per-network MAC，必须清除，否则 SDK 直接拒绝创建请求。
func TestSanitizeCreateConfig_低版本API清理bridge模式下的MAC(t *testing.T) {
	config := &container.Config{}
	hostConfig := &container.HostConfig{NetworkMode: "bridge"}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"bridge": {MacAddress: "02:42:ac:11:00:02"},
		},
	}

	SanitizeCreateConfig("test", "1.43", config, hostConfig, networkingConfig)

	if got := networkingConfig.EndpointsConfig["bridge"].MacAddress; got != "" {
		t.Errorf("API 1.43 下 MAC 应被清空，实际: %q", got)
	}
}

// 版本未知（空串）时保守处理：不动用户的有效配置。
func TestSanitizeCreateConfig_版本未知时保留MAC(t *testing.T) {
	config := &container.Config{}
	hostConfig := &container.HostConfig{NetworkMode: "bridge"}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"bridge": {MacAddress: "02:42:ac:11:00:02"},
		},
	}

	SanitizeCreateConfig("test", "", config, hostConfig, networkingConfig)

	if got := networkingConfig.EndpointsConfig["bridge"].MacAddress; got != "02:42:ac:11:00:02" {
		t.Errorf("版本未知时 MAC 应保留，实际: %q", got)
	}
}

// 配置缺失时不应 panic。
func TestSanitizeCreateConfig_空配置不panic(t *testing.T) {
	SanitizeCreateConfig("test", "1.44", nil, nil, nil)
	SanitizeCreateConfig("test", "1.44", &container.Config{}, nil, nil)
	SanitizeCreateConfig("test", "1.44", &container.Config{}, &container.HostConfig{}, nil)
}
