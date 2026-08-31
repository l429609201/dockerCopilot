package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/notify"
	"github.com/l429609201/dockerCopilot/internal/svc"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// RunRule 执行一条定时任务规则：按类型分发到更新/清理/备份。
// 该函数可被 cron 调度和"立即执行"接口复用（单一职责：只负责编排一条规则的执行）。
func RunRule(svcCtx *svc.ServiceContext, notifier notify.Notifier, rule appconfig.ScheduledUpdateRule) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("定时任务规则[%s]执行 panic 已恢复: %v", rule.Name, r)
		}
	}()

	// 按任务类型分发；空类型按 update 处理，兼容历史数据。
	switch rule.Type {
	case appconfig.RuleTypePrune:
		runPrune(svcCtx, notifier, rule)
		return
	case appconfig.RuleTypeBackup:
		runBackup(svcCtx, notifier, rule)
		return
	}

	// 默认：自动更新容器
	// 解析本规则的有效目标：优先 ContainerTargets（精确到主机）；为空时回退 ContainerNames（视为本地）
	targets := effectiveTargets(rule)
	if notifier != nil && rule.NotifyOnStart {
		notifier.Notify("定时更新开始", fmt.Sprintf("规则「%s」开始执行，容器数：%d", rule.Name, len(targets)))
	}

	// 汇总任务：在任务中心落一条“规则维度”的任务，展示本轮更新了哪些容器/镜像及结果。
	// 与各容器自身的更新任务并存（后者是单容器进度，前者是整轮汇总）。
	summaryTaskID := "sched-update-" + rule.ID
	summaryName := "定时更新·" + rule.Name
	// 显式传本次开始时间：同一规则复用固定 taskID，多次执行会覆盖同一条记录，
	// 若不显式覆盖 StartedAt，UpdateProgress 会沿用上一轮的开始时间导致排序错乱。
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	svcCtx.UpdateProgress(summaryTaskID, svc.TaskProgress{
		TaskID:     summaryTaskID,
		Name:       summaryName,
		Percentage: 0,
		Message:    fmt.Sprintf("开始执行，目标容器 %d 个", len(targets)),
		DetailMsg:  "正在获取容器列表…",
		TaskType:   svc.TaskTypeScheduledUpdate,
		StartedAt:  nowMs,
	})

	// 聚合所有主机的容器，保证远程主机的目标也能被匹配到
	containers, err := utiles.GetAllContainers(svcCtx)
	if err != nil {
		logx.Errorf("定时更新规则[%s]获取容器列表失败: %v", rule.Name, err)
		if notifier != nil && rule.NotifyOnError {
			notifier.Notify("定时更新失败", fmt.Sprintf("规则「%s」获取容器列表失败：%s", rule.Name, err.Error()))
		}
		recordResult(svcCtx, rule.ID, "获取容器列表失败："+err.Error())
		// 汇总任务标记失败结束
		svcCtx.UpdateProgress(summaryTaskID, svc.TaskProgress{
			TaskID:   summaryTaskID, Name: summaryName, Percentage: 100,
			Message:  "获取容器列表失败", DetailMsg: err.Error(),
			TaskType: svc.TaskTypeScheduledUpdate, IsDone: true, Failed: true,
		})
		return
	}
	containers = utiles.CheckImageUpdate(svcCtx, containers)

	// 预编码凭据，供本轮所有容器复用
	// 自适应模式：按每个容器镜像的 registry 自动匹配凭据（在循环内计算）
	autoAuth := rule.RegistryID == utiles.RegistryIDAuto
	registryAuth := ""
	if rule.RegistryID != "" && !autoAuth {
		if cred, ok := svcCtx.AppConfig.FindRegistry(rule.RegistryID); ok {
			if auth, e := utiles.EncodeRegistryAuth(&cred); e == nil {
				registryAuth = auth
			} else {
				logx.Errorf("定时更新规则[%s]编码凭据失败: %v", rule.Name, e)
			}
		}
	}

	// 目标集合：以「主机ID|容器名」为键，精确匹配到具体主机的容器
	targetSet := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		targetSet[targetKey(t.HostID, t.Name)] = struct{}{}
	}

	// 结构化明细：携带容器 ID/主机/镜像/原因，供"查看跳过·失败明细"和"重试全部失败"复用。
	var updatedList, skippedList, failedList []notify.ResultItem
	timeoutSec := svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	// 把 DC 自身排到队尾最后更新。
	// 原因：SelfUpdate 只负责拉镜像并拉起 helper 就立即返回，helper 随后会异步停掉本进程。
	// 若 DC 排在中间，后续容器的更新会在「旧容器已删、新容器未建」时被进程终止，
	// 导致容器彻底丢失（用户反馈：镜像已拉完但新容器没生成）。
	containers = moveSelfToEnd(svcCtx, containers)

	for _, c := range containers {
		// DC 自身已进入自更新交接阶段，本进程随时会被 helper 停止。
		// 此时继续更新其他容器会在中途被打断，直接中止本轮剩余任务。
		if svcCtx.TaskManager.IsSelfUpdating() {
			logx.Infof("定时更新规则[%s]因本程序自更新中止，剩余容器本轮跳过", rule.Name)
			break
		}
		name := containerName(c)
		hostID := normalizeHostID(c.HostID)
		if _, want := targetSet[targetKey(hostID, name)]; !want {
			continue
		}
		// 展示名：非本地容器带主机名，便于通知区分
		dispName := name
		if hostID != appconfig.DockerHostLocalID && c.HostName != "" {
			dispName = fmt.Sprintf("%s@%s", name, c.HostName)
		}
		// 策略：仅在有更新时执行
		if rule.OnlyWhenUpdate && !c.Update {
			skippedList = append(skippedList, notify.ResultItem{
				HostID: hostID, ID: c.ID, Name: dispName, Image: c.Image, Reason: "已是最新版本",
			})
			continue
		}
		// 策略：跳过无 tag 或 digest 形式镜像
		if rule.SkipInvalidTag && !isValidImageRef(c.Image) {
			logx.Infof("定时更新规则[%s]跳过非法镜像标签容器 %s (%s)", rule.Name, dispName, c.Image)
			skippedList = append(skippedList, notify.ResultItem{
				HostID: hostID, ID: c.ID, Name: dispName, Image: c.Image,
				Reason: "镜像标签无效: " + shortImage(c.Image),
			})
			continue
		}
		// 自适应模式：按当前容器镜像匹配凭据；否则用预编码的复用凭据
		auth := registryAuth
		if autoAuth {
			auth = utiles.MatchRegistryAuthByImage(svcCtx.AppConfig, c.Image)
		}
		item := notify.ResultItem{HostID: hostID, ID: c.ID, Name: dispName, Image: c.Image}
		if runOne(svcCtx, hostID, c.ID, name, c.Image, !rule.KeepOldContainer, auth, timeoutSec) {
			item.Reason = "镜像: " + shortImage(c.Image)
			updatedList = append(updatedList, item)
		} else {
			item.Reason = "更新失败: " + shortImage(c.Image)
			failedList = append(failedList, item)
		}
	}
	updated, skipped, failed := len(updatedList), len(skippedList), len(failedList)

	summary := fmt.Sprintf("更新 %d，跳过 %d，失败 %d", updated, skipped, failed)
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时更新规则[%s]执行完成：%s", rule.Name, summary)

	// 汇总任务收尾：把本轮更新/跳过/失败的容器明细铺进 DetailMsg（前端任务中心
	// 完整换行显示），failed>0 且无成功时标红为失败态。前端零改动即可查看。
	svcCtx.UpdateProgress(summaryTaskID, svc.TaskProgress{
		TaskID:     summaryTaskID,
		Name:       summaryName,
		Percentage: 100,
		Message:    summary,
		DetailMsg:  buildUpdateDetail(updatedList, skippedList, failedList),
		TaskType:   svc.TaskTypeScheduledUpdate,
		IsDone:     true,
		Failed:     failed > 0 && updated == 0,
	})

	// 保存本次执行明细到公共 result store：供 Bot 端「查看跳过/失败明细」和「重试全部失败」取用。
	result := &notify.RuleUpdateResult{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		KeepOld:  rule.KeepOldContainer,
		Updated:  updatedList,
		Skipped:  skippedList,
		Failed:   failedList,
	}
	notify.SaveRuleUpdateResult(result)

	if notifier == nil || !rule.NotifyOnDone {
		return
	}

	// 优先：带交互式键盘的完成通知（正文只留统计+已更新列表，跳过/失败改按钮按需查看）。
	if kbNotifier, ok := notifier.(notify.RuleResultNotifier); ok {
		kbNotifier.NotifyRuleResult(result)
		return
	}

	// 回退：纯文本通知，正文铺开三段明细（渠道未实现键盘能力时）。
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("规则「%s」执行完成\n\n", rule.Name))
	msg.WriteString(fmt.Sprintf("📊 统计：更新 %d 个，跳过 %d 个，失败 %d 个\n", updated, skipped, failed))
	if len(updatedList) > 0 {
		msg.WriteString("\n✅ 已更新：\n")
		for _, item := range updatedList {
			msg.WriteString(fmt.Sprintf("  • %s (%s)\n", item.Name, item.Reason))
		}
	}
	if len(skippedList) > 0 {
		msg.WriteString("\n⏭️ 已跳过：\n")
		for _, item := range skippedList {
			msg.WriteString(fmt.Sprintf("  • %s (%s)\n", item.Name, item.Reason))
		}
	}
	if len(failedList) > 0 {
		msg.WriteString("\n❌ 更新失败：\n")
		for _, item := range failedList {
			msg.WriteString(fmt.Sprintf("  • %s (%s)\n", item.Name, item.Reason))
		}
	}
	notifier.Notify("定时更新完成", msg.String())
}

// buildUpdateDetail 把本轮更新/跳过/失败的容器明细拼成多行文本，
// 供任务中心汇总任务的 DetailMsg 完整展示（前端按 whitespace-pre-wrap 换行渲染）。
// 每类最多列出 30 条，超出用省略提示，避免超长文本拖慢渲染。
func buildUpdateDetail(updated, skipped, failed []notify.ResultItem) string {
	const maxPerGroup = 30
	writeGroup := func(sb *strings.Builder, title string, items []notify.ResultItem) {
		if len(items) == 0 {
			return
		}
		sb.WriteString(title)
		sb.WriteString("\n")
		for i, it := range items {
			if i >= maxPerGroup {
				sb.WriteString(fmt.Sprintf("  … 及其余 %d 个\n", len(items)-maxPerGroup))
				break
			}
			sb.WriteString(fmt.Sprintf("  • %s（%s）\n", it.Name, it.Reason))
		}
	}
	var sb strings.Builder
	writeGroup(&sb, "✅ 已更新：", updated)
	writeGroup(&sb, "⏭️ 已跳过：", skipped)
	writeGroup(&sb, "❌ 更新失败：", failed)
	if sb.Len() == 0 {
		return "本轮无匹配容器"
	}
	return strings.TrimRight(sb.String(), "\n")
}

// moveSelfToEnd 将 DC 自身所在容器移动到列表末尾，保证它最后一个更新。
// 判定条件与 runOne 中走 SelfUpdate 的条件保持一致：仅本地主机 + 命中自身。
// 保持其余容器的原有相对顺序，避免影响既有更新次序。
func moveSelfToEnd(svcCtx *svc.ServiceContext, containers []MyType.Container) []MyType.Container {
	selfIdx := -1
	for i, c := range containers {
		if normalizeHostID(c.HostID) == appconfig.DockerHostLocalID && utiles.IsSelfContainer(svcCtx, c.ID) {
			selfIdx = i
			break
		}
	}
	// 未命中自身，或本来就在末尾，无需调整
	if selfIdx < 0 || selfIdx == len(containers)-1 {
		return containers
	}
	self := containers[selfIdx]
	reordered := make([]MyType.Container, 0, len(containers))
	reordered = append(reordered, containers[:selfIdx]...)
	reordered = append(reordered, containers[selfIdx+1:]...)
	reordered = append(reordered, self)
	logx.Infof("定时更新：已将本程序容器 %s 调整到队尾最后更新", containerName(self))
	return reordered
}

// runOne 提交单个容器的更新任务并等待其结束，返回是否成功。
// hostID 指定容器所属 Docker 主机（多 Docker 管理），空表示本地。
func runOne(svcCtx *svc.ServiceContext, hostID, id, name, image string, delOld bool, registryAuth string, timeoutSec int) bool {
	taskID := uuid.New().String()
	done := make(chan bool, 1)
	startErr := svcCtx.TaskManager.TryStart(taskID, id, svc.TaskTypeScheduledUpdate, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		// 本地容器命中本程序自身时走辅助容器方案，避免自己停自己卡死。
		// 远程主机的容器不涉及自身，直接在目标主机上执行更新。
		if normalizeHostID(hostID) == appconfig.DockerHostLocalID && utiles.IsSelfContainer(svcCtx, id) {
			e := utiles.SelfUpdate(ctxWithTimeout, svcCtx, id, name, image, delOld, taskID, registryAuth)
			done <- e == nil
			return
		}
		e := utiles.UpdateContainerOnHost(ctxWithTimeout, svcCtx, hostID, id, name, image, delOld, taskID, registryAuth)
		done <- e == nil
	})
	if startErr != nil {
		logx.Errorf("定时更新容器 %s 提交失败: %v", name, startErr)
		return false
	}
	return <-done
}

// targetKey 生成「主机ID|容器名」的匹配键。
func targetKey(hostID, name string) string {
	return normalizeHostID(hostID) + "|" + name
}

// normalizeHostID 归一化主机ID：空视为本地。
func normalizeHostID(hostID string) string {
	if hostID == "" {
		return appconfig.DockerHostLocalID
	}
	return hostID
}

// effectiveTargets 解析规则的有效更新目标。
// 优先使用 ContainerTargets（精确到主机）；为空时回退 ContainerNames（视为本地容器），兼容历史规则。
func effectiveTargets(rule appconfig.ScheduledUpdateRule) []appconfig.ContainerTarget {
	if len(rule.ContainerTargets) > 0 {
		out := make([]appconfig.ContainerTarget, 0, len(rule.ContainerTargets))
		for _, t := range rule.ContainerTargets {
			if t.Name == "" {
				continue
			}
			out = append(out, appconfig.ContainerTarget{HostID: normalizeHostID(t.HostID), Name: t.Name})
		}
		return out
	}
	out := make([]appconfig.ContainerTarget, 0, len(rule.ContainerNames))
	for _, n := range rule.ContainerNames {
		if n == "" {
			continue
		}
		out = append(out, appconfig.ContainerTarget{HostID: appconfig.DockerHostLocalID, Name: n})
	}
	return out
}

// recordResult 将执行结果写回规则记录。
func recordResult(svcCtx *svc.ServiceContext, ruleID, result string) {
	_ = svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		for i := range cfg.ScheduledUpdates {
			if cfg.ScheduledUpdates[i].ID == ruleID {
				cfg.ScheduledUpdates[i].LastRunAt = time.Now().UnixNano() / int64(time.Millisecond)
				cfg.ScheduledUpdates[i].LastResult = result
				break
			}
		}
		return nil
	})
}

// containerName 提取容器主名称（去掉前导斜杠）。
func containerName(c MyType.Container) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return ""
}

// isValidImageRef 判断镜像引用是否可用于更新（有名有 tag，非 sha256 digest）。
func isValidImageRef(image string) bool {
	if image == "" || strings.HasPrefix(image, "sha256:") {
		return false
	}
	return strings.Contains(image, ":")
}

// shortImage 返回镜像的简短显示名称，去掉 registry 前缀。
// 例如：docker.io/library/nginx:latest → nginx:latest
func shortImage(image string) string {
	// 移除常见 registry 前缀
	image = strings.TrimPrefix(image, "docker.io/")
	image = strings.TrimPrefix(image, "docker.io/library/")

	// 如果还有多级路径，只保留最后两段（名称:标签）
	parts := strings.Split(image, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return image
}
