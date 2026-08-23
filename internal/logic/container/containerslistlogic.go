package container

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/utiles"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ContainersListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

type Info struct {
	Id          string `json:"id"`
	Status      string `json:"status"`
	Name        string `json:"name"`
	UsingImage  string `json:"usingImage"`
	CreateImage string `json:"createImage"`
	CreateTime  string `json:"createTime"`
	RunningTime string `json:"runningTime"`
	HaveUpdate  bool   `json:"haveUpdate"`
	// Ports 暴露到宿主机的端口列表（仅含有 PublicPort 的），供前端抓取站点 favicon
	Ports []int `json:"ports"`
	// NetworkMode 网络模式（如 host / bridge / 自定义网络名），供前端判断 host 模式
	NetworkMode string `json:"networkMode"`
	// ExposedPorts 容器内暴露的端口（来自镜像/配置）。host 网络模式下等同于宿主机端口，
	// 供前端在无端口映射时探测站点 favicon。
	ExposedPorts []int `json:"exposedPorts"`
}

func NewContainersListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ContainersListLogic {
	return &ContainersListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ContainersListLogic) ContainersList() (resp *types.Resp, err error) {
	// 获取所有容器（包括停止的容器）
	resp = &types.Resp{}
	list, err := utiles.GetContainerList(l.svcCtx)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	resp.Msg = "success"
	var containerInfoList []Info
	list = utiles.CheckImageUpdate(l.svcCtx, list)
	for _, v := range list {
		var containerInfo Info
		containerInfo.Id = v.ID
		containerInfo.Status = v.State
		if len(v.Names) > 0 {
			ContainerName := v.Names[0][1:]
			containerInfo.Name = ContainerName
		} else {
			containerInfo.Name = "get container name error"
			l.Error("get container name error" + v.ID)
		}
		if v.Image != "" {
			containerInfo.UsingImage = v.Image
		} else {
			containerInfo.UsingImage = v.ImageID
			l.Error("image dont have name" + v.ID)
		}
		containerInspect, err := utiles.GetContainerInspect(l.svcCtx, v.ID)
		if err != nil {
			containerInfo.CreateImage = ""
			l.Error("get image name error" + v.ID)
		}
		containerInfo.CreateImage = containerInspect.Config.Image
		t := time.Unix(v.Created, 0)
		containerInfo.CreateTime = t.Format("2006-01-02 15:04:05")
		containerInfo.RunningTime = v.Status
		containerInfo.HaveUpdate = v.Update
		// 收集暴露到宿主机的公共端口并去重，供前端探测站点 favicon
		seenPorts := make(map[int]struct{})
		for _, p := range v.Ports {
			if p.PublicPort == 0 {
				continue
			}
			port := int(p.PublicPort)
			if _, ok := seenPorts[port]; ok {
				continue
			}
			seenPorts[port] = struct{}{}
			containerInfo.Ports = append(containerInfo.Ports, port)
		}
		// 网络模式与暴露端口：host 模式下容器内端口即宿主机端口，供前端探测图标
		if containerInspect.HostConfig != nil {
			containerInfo.NetworkMode = string(containerInspect.HostConfig.NetworkMode)
		}
		if containerInspect.Config != nil {
			seenExposed := make(map[int]struct{})
			for portProto := range containerInspect.Config.ExposedPorts {
				numStr := string(portProto)
				if idx := strings.IndexByte(numStr, '/'); idx >= 0 {
					numStr = numStr[:idx]
				}
				n, e := strconv.Atoi(numStr)
				if e != nil || n <= 0 {
					continue
				}
				if _, ok := seenExposed[n]; ok {
					continue
				}
				seenExposed[n] = struct{}{}
				containerInfo.ExposedPorts = append(containerInfo.ExposedPorts, n)
			}
		}
		containerInfoList = append(containerInfoList, containerInfo)
	}
	resp.Data = containerInfoList
	return resp, nil
}
