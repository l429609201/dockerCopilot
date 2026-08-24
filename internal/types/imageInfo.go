package types

import (
	"github.com/docker/docker/api/types/image"
)

type Image struct {
	image.Summary
	ImageName  string `json:"imageName"`
	ImageTag   string `json:"imageTag"`
	InUsed     bool   `json:"inUsed"`
	SizeFormat string `json:"sizeFormat"`
	// HostID / HostName 标记该镜像所属的 Docker 主机（多 Docker 管理）。
	// 本地主机 HostID 为 "local"。
	HostID   string `json:"hostId,omitempty"`
	HostName string `json:"hostName,omitempty"`
}
