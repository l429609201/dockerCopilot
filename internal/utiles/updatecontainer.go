package utiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerMsgType "github.com/docker/docker/pkg/jsonmessage"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateContainer 兼容旧签名的入口（本地主机），内部使用后台 context 调用带 context 版本。
func UpdateContainer(serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	return UpdateContainerWithContext(context.Background(), serviceContext, id, name, imageNameAndTag, delOldContainer, taskID)
}

// UpdateContainerWithContext 支持取消与超时的容器更新流程（匿名拉取，本地主机，兼容旧签名）。
func UpdateContainerWithContext(ctx context.Context, serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	return UpdateContainerOnHost(ctx, serviceContext, "", id, name, imageNameAndTag, delOldContainer, taskID, "")
}

// UpdateContainerWithAuth 支持取消、超时与 Registry 认证的容器更新流程（本地主机，兼容旧签名）。
func UpdateContainerWithAuth(ctx context.Context, serviceContext *svc.ServiceContext, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string, registryAuth string) error {
	return UpdateContainerOnHost(ctx, serviceContext, "", id, name, imageNameAndTag, delOldContainer, taskID, registryAuth)
}

// UpdateContainerOnHost 在指定 Docker 主机上执行容器更新流程。hostID 为空表示本地。
// registryAuth 为 base64(JSON) 编码的认证信息，空串表示匿名拉取。
// 在关键步骤前检查 ctx 是否已取消，取消时立即中止并标记任务失败，
// 从而支持前端/机器人主动取消长时间运行的更新任务。
func UpdateContainerOnHost(ctx context.Context, serviceContext *svc.ServiceContext, hostID string, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string, registryAuth string) error {
	// 按目标主机取 client，未找到回退本地；后续流程统一用此 cli
	cli, ok := serviceContext.DockerManager.GetClient(hostID)
	if !ok || cli == nil {
		cli = serviceContext.DockerClient
	}
	serviceContext.UpdateProgress(taskID, svc.TaskProgress{
		TaskID:     taskID,
		Percentage: 0,
		Name:       name,
		Message:    "正在连接Docker",
		DetailMsg:  "正在连接Docker",
		IsDone:     false,
		TaskType:   svc.TaskTypeContainerUpdate,
		ResourceID: id,
	})
	var oldTaskProgress, result = serviceContext.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	timeout := 10
	signal := "SIGINT"

	// 更新开始前先检查是否已被取消
	if err := ctx.Err(); err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "任务已取消", err)
		return err
	}

	serviceContext.UpdateProgress(taskID, oldTaskProgress)
	oldTaskProgress.Message = "正在拉取新镜像"
	oldTaskProgress.Percentage = 10
	oldTaskProgress.DetailMsg = "正在拉取新镜像"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)
	cli.NegotiateAPIVersion(ctx)
	// 携带凭据拉取私有镜像；registryAuth 为空时等价于匿名拉取
	reader, err := cli.ImagePull(ctx, imageNameAndTag, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "拉取镜像失败", err)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	err = decodePullResp(ctx, reader, serviceContext, taskID)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "拉取镜像失败", err)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	oldTaskProgress, result = serviceContext.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	oldTaskProgress.Message = "拉取镜像成功"
	oldTaskProgress.DetailMsg = "拉取镜像成功"

	// 【特殊处理】如果是 DC 自我更新，交给辅助容器处理
	// 检测逻辑：对比当前容器ID与要更新的容器ID
	selfID := os.Getenv("HOSTNAME") // Docker 容器内 HOSTNAME 通常是容器ID的短格式
	if selfID != "" && strings.HasPrefix(id, selfID) {
		logx.Info("检测到自我更新，启动辅助容器接管")
		return SelfUpdate(ctx, serviceContext, id, name, imageNameAndTag, delOldContainer, taskID, registryAuth)
	}

	// 【Misaka 方式】先停止删除旧容器，再用原名创建新容器
	// 优点：无需重命名，避免重命名失败；逻辑更简单
	oldTaskProgress.Percentage = 30
	oldTaskProgress.Message = "正在获取容器配置"
	oldTaskProgress.DetailMsg = "正在获取容器配置"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 获取旧容器配置
	inspectedContainer, err := cli.ContainerInspect(context.Background(), id)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "获取容器信息失败", err)
		logx.Error("获取容器信息失败" + err.Error())
		return err
	}

	// 【增强】收集旧镜像信息（SHA256、大小）
	oldImageInfo, _, err := cli.ImageInspectWithRaw(context.Background(), inspectedContainer.Image)
	if err != nil {
		logx.Errorf("获取旧镜像信息失败: %v，继续更新流程", err)
	} else {
		// 提取 SHA256（去掉 sha256: 前缀）
		oldDigest := strings.TrimPrefix(oldImageInfo.ID, "sha256:")
		oldTaskProgress.OldImageDigest = oldDigest
		oldTaskProgress.OldImageSize = oldImageInfo.Size
		logx.Infof("旧镜像信息 - Digest: %s, Size: %d bytes", oldDigest, oldImageInfo.Size)
	}

	// 【增强】解析镜像名称和标签
	imageNameParts := strings.Split(imageNameAndTag, ":")
	if len(imageNameParts) == 2 {
		oldTaskProgress.ImageName = imageNameParts[0]
		oldTaskProgress.ImageTag = imageNameParts[1]
	} else {
		oldTaskProgress.ImageName = imageNameAndTag
		oldTaskProgress.ImageTag = "latest"
	}

	// 准备新容器配置（使用新镜像）
	inspectedContainer.Config.Hostname = ""
	inspectedContainer.Config.Image = imageNameAndTag
	inspectedContainer.Image = imageNameAndTag
	config := inspectedContainer.Config
	hostConfig := inspectedContainer.HostConfig
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: inspectedContainer.NetworkSettings.Networks,
	}

	oldTaskProgress.Percentage = 40
	oldTaskProgress.Message = "正在停止旧容器"
	oldTaskProgress.DetailMsg = "正在停止旧容器"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 从停止旧容器开始进入关键区：此后不再响应取消/超时
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	err = cli.ContainerStop(context.Background(), id, stopOptions)
	if err != nil {
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "停止旧容器失败", err)
		return err
	}

	oldTaskProgress.Percentage = 50
	oldTaskProgress.Message = "正在删除旧容器"
	oldTaskProgress.DetailMsg = "正在删除旧容器（释放容器名）"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 删除旧容器（释放容器名，如果配置要求保留则重命名）
	// backupName 用于失败回滚时恢复（仅 !delOldContainer 时有值）
	var backupName string
	if delOldContainer {
		err = cli.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true})
		if err != nil {
			markTaskFailed(serviceContext, taskID, &oldTaskProgress, "删除旧容器失败", err)
			return err
		}
	} else {
		// 保留旧容器：重命名为带时间戳的备份名（统一 {name}-old-{时间戳} 格式，供 CleanupOldBackups 识别）
		currentDate := time.Now().Format("20060102150405")
		backupName = name + "-old-" + currentDate
		err = cli.ContainerRename(context.Background(), id, backupName)
		if err != nil {
			markTaskFailed(serviceContext, taskID, &oldTaskProgress, "重命名旧容器失败", err)
			return err
		}
		logx.Infof("旧容器已重命名为: %s", backupName)
	}

	oldTaskProgress.Percentage = 70
	oldTaskProgress.Message = "正在创建新容器"
	oldTaskProgress.DetailMsg = fmt.Sprintf("使用原名: %s", name)
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 用原名创建新容器（旧容器已删除或重命名，名称可用）
	newContainerResp, err := cli.ContainerCreate(context.Background(), config, hostConfig, networkingConfig, nil, name)
	if err != nil {
		// 【修复】创建失败时尝试回滚旧容器（避免用户容器永久丢失）
		logx.Errorf("创建新容器失败，尝试回滚旧容器: %v", err)
		if !delOldContainer && backupName != "" {
			// 旧容器被重命名为备份，尝试改回原名并重启
			logx.Infof("尝试将备份容器 %s 恢复为原名 %s", backupName, name)
			if renameErr := cli.ContainerRename(context.Background(), backupName, name); renameErr != nil {
				logx.Errorf("恢复容器名失败: %v，用户需手动重命名 %s", renameErr, backupName)
			} else if startErr := cli.ContainerStart(context.Background(), id, container.StartOptions{}); startErr != nil {
				logx.Errorf("重启旧容器失败: %v", startErr)
			} else {
				logx.Info("✅ 已成功回滚并重启旧容器")
			}
		}
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "创建新容器失败（已尝试回滚旧容器）", err)
		return err
	}
	newContainerID := newContainerResp.ID

	oldTaskProgress.Percentage = 90
	oldTaskProgress.Message = "正在启动新容器"
	oldTaskProgress.DetailMsg = "正在启动新容器"
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 启动新容器
	err = cli.ContainerStart(context.Background(), newContainerID, container.StartOptions{})
	if err != nil {
		// 【修复】启动失败时回滚：删除失败的新容器 + 恢复旧容器
		logx.Errorf("启动新容器失败，尝试回滚: %v", err)
		_ = cli.ContainerRemove(context.Background(), newContainerID, container.RemoveOptions{Force: true})
		if !delOldContainer && backupName != "" {
			// 旧容器被重命名为备份，尝试改回原名并重启
			logx.Infof("尝试将备份容器 %s 恢复为原名 %s", backupName, name)
			if renameErr := cli.ContainerRename(context.Background(), backupName, name); renameErr != nil {
				logx.Errorf("恢复容器名失败: %v，用户需手动重命名 %s", renameErr, backupName)
			} else if startErr := cli.ContainerStart(context.Background(), id, container.StartOptions{}); startErr != nil {
				logx.Errorf("重启旧容器失败: %v", startErr)
			} else {
				logx.Info("✅ 已成功回滚并重启旧容器")
			}
		}
		markTaskFailed(serviceContext, taskID, &oldTaskProgress, "启动新容器失败（已尝试回滚旧容器）", err)
		return err
	}

	// 【增强】收集新镜像信息（SHA256、大小）
	newImageInfo, _, err := cli.ImageInspectWithRaw(context.Background(), imageNameAndTag)
	if err != nil {
		logx.Errorf("获取新镜像信息失败: %v，但容器已成功启动", err)
	} else {
		// 提取 SHA256（去掉 sha256: 前缀）
		newDigest := strings.TrimPrefix(newImageInfo.ID, "sha256:")
		oldTaskProgress.NewImageDigest = newDigest
		oldTaskProgress.NewImageSize = newImageInfo.Size
		logx.Infof("新镜像信息 - Digest: %s, Size: %d bytes", newDigest, newImageInfo.Size)
	}

	oldTaskProgress.Message = "更新成功"
	oldTaskProgress.DetailMsg = "更新成功"
	oldTaskProgress.Percentage = 100
	oldTaskProgress.IsDone = true
	serviceContext.UpdateProgress(taskID, oldTaskProgress)

	// 更新成功后清理历史 -old- 备份，避免无限累积：
	// delOld=true 本次未留备份 → 清空全部历史；delOld=false 本次留了 1 个 → 只保留最新 1 个。
	keep := 0
	if !delOldContainer {
		keep = 1
	}
	CleanupOldBackups(cli, name, keep)
	return nil
}

// markTaskFailed 统一将任务标记为失败结束，避免各处重复样板代码。
func markTaskFailed(svcCtx *svc.ServiceContext, taskID string, progress *svc.TaskProgress, message string, cause error) {
	progress.Message = message
	if cause != nil {
		progress.DetailMsg = cause.Error()
	} else {
		progress.DetailMsg = message
	}
	progress.IsDone = true
	progress.Failed = true
	if cause != nil && errors.Is(cause, context.Canceled) {
		progress.Canceled = true
	}
	svcCtx.UpdateProgress(taskID, *progress)
}

// layerState 记录单层的中间状态，供聚合与主进度换算使用。
type layerState struct {
	order   int    // 首次出现的顺序，保证展示稳定
	status  string
	current int64
	total   int64
}

// pullFraction 计算所有已知层的整体下载占比（0.0~1.0）。
// 以字节为权重：已完成层按其 total 计满，未知 total 的已完成层用已见的平均层大小估算，
// 避免因层的 total 尚未上报而导致占比抖动。总权重为 0 时返回 0。
func pullFraction(m map[string]*layerState) float64 {
	var totalBytes int64
	var knownTotalCount int64
	// 先求已知 total 的层的平均大小，供 total 未知的层估算权重
	for _, st := range m {
		if st.total > 0 {
			totalBytes += st.total
			knownTotalCount++
		}
	}
	avg := int64(0)
	if knownTotalCount > 0 {
		avg = totalBytes / knownTotalCount
	}
	var weightSum, progressSum float64
	for _, st := range m {
		w := st.total
		if w <= 0 {
			w = avg // total 未知时用平均层大小兜底
		}
		if w <= 0 {
			w = 1 // 仍未知（首个层尚无任何字节信息）时给最小权重
		}
		weightSum += float64(w)
		if isLayerDone(st.status) {
			progressSum += float64(w)
		} else if st.total > 0 && st.current > 0 {
			frac := float64(st.current) / float64(st.total)
			if frac > 1 {
				frac = 1
			}
			progressSum += float64(w) * frac
		}
	}
	if weightSum <= 0 {
		return 0
	}
	f := progressSum / weightSum
	if f > 1 {
		f = 1
	}
	return f
}

// clampMonotonic 将候选百分比钳制到 [lo, hi] 区间，并保证不低于当前值（单调不回退）。
// 用于消除拉取阶段因层数/字节上报次序造成的进度回跳。
func clampMonotonic(current, candidate, lo, hi int) int {
	if candidate < lo {
		candidate = lo
	}
	if candidate > hi {
		candidate = hi
	}
	if candidate < current {
		return current // 只增不减
	}
	return candidate
}

// isLayerDone 判断层状态是否已完成（用于主进度按完成层数换算）。
func isLayerDone(status string) bool {
	switch status {
	case "Pull complete", "Already exists", "Download complete":
		return true
	}
	return false
}

// buildLayers 将层状态 map 按首次出现顺序整理为切片，并计算每层百分比。
func buildLayers(m map[string]*layerState) ([]svc.LayerProgress, int, int) {
	ordered := make([]*layerState, 0, len(m))
	idByPtr := make(map[*layerState]string, len(m))
	for id, st := range m {
		ordered = append(ordered, st)
		idByPtr[st] = id
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].order < ordered[j].order })
	layers := make([]svc.LayerProgress, 0, len(ordered))
	doneCount := 0
	for _, st := range ordered {
		pct := 0
		if isLayerDone(st.status) {
			pct = 100
			doneCount++
		} else if st.total > 0 {
			pct = int(st.current * 100 / st.total)
			if pct > 99 {
				pct = 99 // 未到完成态最多显示 99%，避免视觉上先到 100 再跳完成
			}
		}
		layers = append(layers, svc.LayerProgress{
			ID:         idByPtr[st],
			Status:     st.status,
			Current:    st.current,
			Total:      st.total,
			Percentage: pct,
		})
	}
	return layers, doneCount, len(ordered)
}

func decodePullResp(taskCtx context.Context, reader io.Reader, svcCtx *svc.ServiceContext, taskID string) (err error) {
	decoder := json.NewDecoder(reader)
	var oldTaskProgress, result = svcCtx.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	// 各层进度聚合表：key 为层短ID
	layerMap := make(map[string]*layerState)
	layerSeq := 0
	for {
		// 每次读取前检查是否被取消，实现拉取阶段的可中断
		if cerr := taskCtx.Err(); cerr != nil {
			oldTaskProgress.Message = "拉取镜像已取消"
			oldTaskProgress.DetailMsg = cerr.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			oldTaskProgress.Canceled = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			return fmt.Errorf("拉取镜像已取消: %w", cerr)
		}
		var msg dockerMsgType.JSONMessage
		if err = decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = err.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Failed to decode pull image response: %s", err)
			return fmt.Errorf("拉取镜像失败: %w", err)
		}
		// Print the progress or error information from the response
		if msg.Error != nil {
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = msg.Error.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			oldTaskProgress.Failed = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Error: %s", msg.Error)
			return fmt.Errorf("拉取镜像失败: %w", msg.Error)
		} else {
			var formattedMsg string
			if msg.Progress != nil {
				formattedMsg = fmt.Sprintf("进度%s: %s", msg.Status, msg.Progress.String())
			} else {
				formattedMsg = fmt.Sprintf("进度%s", msg.Status)
			}
			oldTaskProgress.DetailMsg = formattedMsg

			// 按层聚合进度：msg.ID 为层短ID（无ID的是全局状态行，跳过分层聚合）
			if msg.ID != "" {
				st, ok := layerMap[msg.ID]
				if !ok {
					st = &layerState{order: layerSeq}
					layerSeq++
					layerMap[msg.ID] = st
				}
				st.status = msg.Status
				if msg.Progress != nil {
					st.current = msg.Progress.Current
					st.total = msg.Progress.Total
				}
				layers, _, _ := buildLayers(layerMap)
				oldTaskProgress.Layers = layers
				// 主进度：拉取阶段映射到 10~30% 区间。
				// 用「已下载字节/总字节」的整体占比驱动，而非「完成层数/总层数」——
				// 后者因 Docker 逐层上报、总层数动态增大而导致分母突变、百分比回跳。
				pulled := pullFraction(layerMap)
				oldTaskProgress.Percentage = clampMonotonic(oldTaskProgress.Percentage, 10+int(pulled*20), 10, 30)
			}
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Infof("拉取镜像进度\t %s: %s\n", msg.Status, msg.Progress)
		}
	}
}
