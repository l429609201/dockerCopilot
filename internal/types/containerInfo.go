package types

import (
	docker "github.com/docker/docker/api/types"
)

type Container struct {
	docker.Container
	Update bool `json:"Update"`
	// HostID / HostName 标记该容器所属的 Docker 主机（多 Docker 管理）。
	// 空表示本地主机，兼容历史行为。
	HostID   string `json:"hostId,omitempty"`
	HostName string `json:"hostName,omitempty"`
}
