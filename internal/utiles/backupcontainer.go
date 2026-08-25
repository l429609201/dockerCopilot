package utiles

import (
	"context"
	"encoding/json"
	dockerBackend "github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/network"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
	"path/filepath"
	"time"
)

// BackupContainer 备份本地主机所有容器配置（保留原签名，兼容既有调用）。
func BackupContainer(ctx *svc.ServiceContext) error {
	return BackupContainerOnHost(ctx, "")
}

// BackupContainerOnHost 备份指定 Docker 主机的所有容器配置为 JSON 文件。
// hostID 为空表示本地；非本地主机的备份文件名会带主机ID后缀以便区分。
func BackupContainerOnHost(ctx *svc.ServiceContext, hostID string) error {
	// 归一化：空 hostID 视为本地主机
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	// 定位目标主机客户端；不可用则回退本地
	cli, ok := ctx.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		cli = ctx.DockerClient
	}
	containerList, err := GetContainerListFromHost(ctx, hostID)
	if err != nil {
		return err
	}
	var backupList []dockerBackend.ContainerCreateConfig
	for i, v := range containerList {
		containerID := containerList[i].ID
		cli.NegotiateAPIVersion(context.TODO())
		inspectedContainer, err := cli.ContainerInspect(context.TODO(), containerID)
		if err != nil {
			logx.Error("获取容器信息失败" + err.Error())
			return err
		}
		var containerName string
		if len(v.Names) > 0 {
			containerName = v.Names[0][1:]
		} else {
			containerName = "get container name error"
			logx.Error("get container name error" + v.ID)
		}
		inspectedContainer.Config.Hostname = ""
		inspectedContainer.Image = inspectedContainer.Config.Image
		config := inspectedContainer.Config
		hostConfig := inspectedContainer.HostConfig
		networkingConfig := &network.NetworkingConfig{
			EndpointsConfig: inspectedContainer.NetworkSettings.Networks,
		}
		createConfig := dockerBackend.ContainerCreateConfig{Config: config, HostConfig: hostConfig, NetworkingConfig: networkingConfig, Name: containerName}
		backupList = append(backupList, createConfig)
	}
	jsonData, err := json.MarshalIndent(backupList, "", "  ")
	if err != nil {
		logx.Error("Error marshalling data:", err)
		return err
	}
	backupDir := os.Getenv("BACKUP_DIR") // 从环境变量中获取备份目录
	if backupDir == "" {
		backupDir = "/data/backups" // 如果环境变量未设置，使用默认值
	}
	_, err = os.Stat(backupDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(backupDir, 0755)
		if err != nil {
			logx.Error("Error creating backup directory:", err)
			return err
		}
	}
	currentDate := time.Now().Format("2006-01-02")
	// 非本地主机的备份文件名带主机ID后缀，避免多主机同日备份互相覆盖
	fileName := "backup-" + currentDate + ".json"
	if hostID != "" && hostID != "local" {
		fileName = "backup-" + hostID + "-" + currentDate + ".json"
	}
	fullPath := filepath.Join(backupDir, fileName)
	err = os.WriteFile(fullPath, jsonData, 0644)
	if err != nil {
		logx.Error("Error writing to file:", err)
		return err
	}
	return nil
}
