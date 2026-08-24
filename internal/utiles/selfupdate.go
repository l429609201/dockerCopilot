package utiles

import (
	"context"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// Helper 模式相关的环境变量键名。
// 主容器检测到"更新目标是自己"时，会用新镜像启动一个带这些变量的辅助容器，
// 辅助容器接管"停旧→建新→启动→删旧"的收尾工作，规避自己停自己导致的卡死。
const (
	EnvHelperMode      = "DC_HELPER_MODE"       // =1 表示以辅助容器模式运行
	EnvHelperTargetID  = "DC_HELPER_TARGET_ID"  // 待更新的主容器ID
	EnvHelperTargetNam = "DC_HELPER_TARGET_NAME" // 主容器名
	EnvHelperImage     = "DC_HELPER_IMAGE"      // 新镜像 name:tag
	EnvHelperDelOld    = "DC_HELPER_DEL_OLD"    // =1 删除旧容器
	EnvHelperTaskID    = "DC_HELPER_TASK_ID"    // 关联的任务ID（仅记录用）
)

// GetSelfContainerID 尽力获取当前进程所在容器的完整/短ID。
// 优先解析 /proc/self/mountinfo 与 /proc/self/cgroup，退回 hostname。
func GetSelfContainerID() string {
	// cgroup v1/v2 路径里通常含 64 位十六进制的容器ID
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		if id := extractDockerID(string(data)); id != "" {
			logx.Infof("📍 从 /proc/self/cgroup 获取容器ID: %s", id)
			return id
		}
	}
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		if id := extractDockerID(string(data)); id != "" {
			logx.Infof("📍 从 /proc/self/mountinfo 获取容器ID: %s", id)
			return id
		}
	}
	// 退回 hostname：Docker 默认将容器ID前12位作为 hostname
	if h, err := os.Hostname(); err == nil {
		logx.Infof("📍 从 hostname 获取容器ID: %s", h)
		return h
	}
	logx.Errorf("❌ 无法获取当前容器ID")
	return ""
}

// extractDockerID 从文本中提取 64 位十六进制的容器ID。
func extractDockerID(text string) string {
	for _, line := range strings.Split(text, "\n") {
		// 常见形如 .../docker/<64hex> 或 .../docker-<64hex>.scope
		for _, seg := range strings.FieldsFunc(line, func(r rune) bool {
			return r == '/' || r == '-' || r == '.' || r == ' '
		}) {
			if len(seg) == 64 && isHex(seg) {
				return seg
			}
		}
	}
	return ""
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// IsSelfContainer 判断给定容器ID是否就是当前程序所在的容器。
// 兼容短ID/长ID：只要互为前缀即认为是同一个。
func IsSelfContainer(svcCtx *svc.ServiceContext, id string) bool {
	self := GetSelfContainerID()
	logx.Infof("🔍 自我更新检测: 目标容器ID=%s, 当前容器ID=%s", id, self)

	if self == "" || id == "" {
		logx.Infof("❌ 自我更新检测失败: self=%s, id=%s", self, id)
		return false
	}

	// 第一步：前缀匹配
	if strings.HasPrefix(id, self) || strings.HasPrefix(self, id) {
		logx.Infof("✅ 检测到自我更新（前缀匹配）: 目标=%s, 自身=%s", id, self)
		return true
	}

	// 第二步：docker inspect 兜底比对
	insp, err := svcCtx.DockerClient.ContainerInspect(context.Background(), id)
	if err == nil {
		short := insp.ID
		if len(short) >= 12 {
			short = short[:12]
		}
		if self == insp.ID || self == short {
			logx.Infof("✅ 检测到自我更新（inspect匹配）: 目标=%s, 自身=%s, 完整ID=%s", id, self, insp.ID)
			return true
		}
	}

	logx.Infof("⚠️ 非自我更新: 目标=%s, 自身=%s", id, self)
	return false
}

// StartHelperContainer 用新镜像启动一个一次性辅助容器，接管主容器的更新收尾。
// 辅助容器：挂载 docker.sock、AutoRemove、带 EnvHelper* 环境变量。
func StartHelperContainer(svcCtx *svc.ServiceContext, targetID, targetName, newImage, taskID string, delOld bool) error {
	ctx := context.Background()
	cli := svcCtx.DockerClient
	cli.NegotiateAPIVersion(ctx)

	delFlag := "0"
	if delOld {
		delFlag = "1"
	}
	env := []string{
		EnvHelperMode + "=1",
		EnvHelperTargetID + "=" + targetID,
		EnvHelperTargetNam + "=" + targetName,
		EnvHelperImage + "=" + newImage,
		EnvHelperDelOld + "=" + delFlag,
		EnvHelperTaskID + "=" + taskID,
	}
	cfg := &container.Config{
		Image: newImage,
		Env:   env,
	}
	// 挂载宿主 docker.sock，让辅助容器能操作 Docker
	hostCfg := &container.HostConfig{
		Binds:      []string{"/var/run/docker.sock:/var/run/docker.sock"},
		AutoRemove: true,
	}
	name := targetName + "-selfupdate-helper"
	created, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		logx.Errorf("创建自更新辅助容器失败: %v", err)
		return err
	}
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		logx.Errorf("启动自更新辅助容器失败: %v", err)
		return err
	}
	logx.Infof("自更新辅助容器已启动: %s", name)
	return nil
}

// pullImageForSelfUpdate 供 SelfUpdate 复用的镜像拉取（带进度）。
func pullImageForSelfUpdate(ctx context.Context, svcCtx *svc.ServiceContext, imageNameAndTag, registryAuth, taskID string) error {
	svcCtx.DockerClient.NegotiateAPIVersion(ctx)
	reader, err := svcCtx.DockerClient.ImagePull(ctx, imageNameAndTag, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		return err
	}
	return decodePullResp(ctx, reader, svcCtx, taskID)
}

// SelfUpdate 处理"更新目标是自己"的场景：先拉好新镜像，再交给辅助容器收尾。
// 关键点：主容器只负责拉镜像和拉起 helper，绝不自己停自己，避免半更新卡死。
func SelfUpdate(ctx context.Context, svcCtx *svc.ServiceContext, id, name, imageNameAndTag string, delOld bool, taskID, registryAuth string) error {
	logx.Infof("🚀🚀🚀 进入 SelfUpdate 流程！容器=%s, 镜像=%s, taskID=%s", name, imageNameAndTag, taskID)

	progress := svc.TaskProgress{
		TaskID:     taskID,
		Name:       name,
		Percentage: 0,
		Message:    "检测到更新目标为本程序，启用辅助容器更新",
		DetailMsg:  "检测到更新目标为本程序，启用辅助容器更新",
		TaskType:   svc.TaskTypeContainerUpdate,
		ResourceID: id,
	}
	svcCtx.UpdateProgress(taskID, progress)

	// 1) 先拉新镜像（此时主进程还活着，可正常显示进度）
	progress.Percentage = 10
	progress.Message = "正在拉取新镜像"
	progress.DetailMsg = "正在拉取新镜像"
	svcCtx.UpdateProgress(taskID, progress)
	if err := pullImageForSelfUpdate(ctx, svcCtx, imageNameAndTag, registryAuth, taskID); err != nil {
		markTaskFailed(svcCtx, taskID, &progress, "拉取镜像失败", err)
		return err
	}

	// 拉取过程中 decodePullResp 会直接改写 store 中的进度（含分层明细），
	// 这里必须重新读回最新值，否则用陈旧的局部变量覆盖会抹掉分层数据并造成进度回跳。
	if latest, ok := svcCtx.GetProgress(taskID); ok {
		progress = latest
	}

	// 2) 用新镜像拉起辅助容器，由它接管停旧/建新/启动/删旧
	// 60% 承接拉取阶段结束时的 30%，保证单调递增
	progress.Percentage = 60
	progress.Message = "镜像就绪，正在启动辅助容器接管更新"
	progress.DetailMsg = "主程序即将被辅助容器重启，请稍候几秒后刷新页面"
	svcCtx.UpdateProgress(taskID, progress)
	if err := StartHelperContainer(svcCtx, id, name, imageNameAndTag, taskID, delOld); err != nil {
		markTaskFailed(svcCtx, taskID, &progress, "启动辅助容器失败", err)
		return err
	}

	// 3) 标记为"已交接"。真正的重启由 helper 完成，本进程稍后会被 helper 停止。
	progress.Percentage = 90
	progress.Message = "辅助容器已接管，正在重启本程序"
	progress.DetailMsg = "辅助容器已接管，正在重启本程序。约10-30秒后请刷新页面"
	progress.IsDone = true
	svcCtx.UpdateProgress(taskID, progress)
	logx.Info("自更新已交接给辅助容器，本进程等待被替换")
	return nil
}
