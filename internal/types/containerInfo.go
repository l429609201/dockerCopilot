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
	// CreateImage 创建容器时使用的镜像名（来自 Config.Image），不受后续 tag 变化影响。
	// 优先使用此字段进行更新操作，避免 Image 字段在镜像更新后变成空字符串或 SHA256 的问题。
	CreateImage string `json:"createImage,omitempty"`
}
