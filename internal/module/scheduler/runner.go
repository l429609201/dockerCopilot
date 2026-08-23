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

// RunRule 执行一条定时更新规则：匹配容器 -> 策略过滤 -> 逐个提交更新任务。
// 该函数可被 cron 调度和"立即执行"接口复用（单一职责：只负责编排一条规则的执行）。
func RunRule(svcCtx *svc.ServiceContext, notifier notify.Notifier, rule appconfig.ScheduledUpdateRule) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("定时更新规则[%s]执行 panic 已恢复: %v", rule.Name, r)
		}
	}()

	if notifier != nil && rule.NotifyOnStart {
		notifier.Notify("定时更新开始", fmt.Sprintf("规则「%s」开始执行，容器数：%d", rule.Name, len(rule.ContainerNames)))
	}

	containers, err := utiles.GetContainerList(svcCtx)
	if err != nil {
		logx.Errorf("定时更新规则[%s]获取容器列表失败: %v", rule.Name, err)
		if notifier != nil && rule.NotifyOnError {
			notifier.Notify("定时更新失败", fmt.Sprintf("规则「%s」获取容器列表失败：%s", rule.Name, err.Error()))
		}
		recordResult(svcCtx, rule.ID, "获取容器列表失败："+err.Error())
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

	// 自动清理：把规则里已不存在于当前容器列表的容器名移除并持久化，
	// 避免规则长期残留已删除的容器名。
	existingNames := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		if n := containerName(c); n != "" {
			existingNames[n] = struct{}{}
		}
	}
	rule.ContainerNames = pruneMissingContainers(svcCtx, rule, existingNames)

	nameSet := make(map[string]struct{}, len(rule.ContainerNames))
	for _, n := range rule.ContainerNames {
		nameSet[n] = struct{}{}
	}

	var updated, skipped, failed int
	var updatedList, skippedList, failedList []string // 记录详细列表
	timeoutSec := svcCtx.Config.Task.PullTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	for _, c := range containers {
		name := containerName(c)
		if _, want := nameSet[name]; !want {
			continue
		}
		// 策略：仅在有更新时执行
		if rule.OnlyWhenUpdate && !c.Update {
			skipped++
			skippedList = append(skippedList, fmt.Sprintf("%s (已是最新版本)", name))
			continue
		}
		// 策略：跳过无 tag 或 digest 形式镜像
		if rule.SkipInvalidTag && !isValidImageRef(c.Image) {
			logx.Infof("定时更新规则[%s]跳过非法镜像标签容器 %s (%s)", rule.Name, name, c.Image)
			skipped++
			skippedList = append(skippedList, fmt.Sprintf("%s (镜像标签无效: %s)", name, shortImage(c.Image)))
			continue
		}
		// 自适应模式：按当前容器镜像匹配凭据；否则用预编码的复用凭据
		auth := registryAuth
		if autoAuth {
			auth = utiles.MatchRegistryAuthByImage(svcCtx.AppConfig, c.Image)
		}
		if runOne(svcCtx, c.ID, name, c.Image, !rule.KeepOldContainer, auth, timeoutSec) {
			updated++
			updatedList = append(updatedList, fmt.Sprintf("%s (镜像: %s)", name, shortImage(c.Image)))
		} else {
			failed++
			failedList = append(failedList, fmt.Sprintf("%s (镜像: %s)", name, shortImage(c.Image)))
		}
	}

	summary := fmt.Sprintf("更新 %d，跳过 %d，失败 %d", updated, skipped, failed)
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时更新规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil && rule.NotifyOnDone {
		// 构建详细消息
		var msg strings.Builder
		msg.WriteString(fmt.Sprintf("规则「%s」执行完成\n\n", rule.Name))
		msg.WriteString(fmt.Sprintf("📊 统计：更新 %d 个，跳过 %d 个，失败 %d 个\n", updated, skipped, failed))

		// 更新成功列表
		if len(updatedList) > 0 {
			msg.WriteString("\n✅ 已更新：\n")
			for _, item := range updatedList {
				msg.WriteString(fmt.Sprintf("  • %s\n", item))
			}
		}

		// 跳过列表（含原因）
		if len(skippedList) > 0 {
			msg.WriteString("\n⏭️ 已跳过：\n")
			for _, item := range skippedList {
				msg.WriteString(fmt.Sprintf("  • %s\n", item))
			}
		}

		// 失败列表
		if len(failedList) > 0 {
			msg.WriteString("\n❌ 更新失败：\n")
			for _, item := range failedList {
				msg.WriteString(fmt.Sprintf("  • %s\n", item))
			}
		}

		notifier.Notify("定时更新完成", msg.String())
	}
}

// runOne 提交单个容器的更新任务并等待其结束，返回是否成功。
func runOne(svcCtx *svc.ServiceContext, id, name, image string, delOld bool, registryAuth string, timeoutSec int) bool {
	taskID := uuid.New().String()
	done := make(chan bool, 1)
	startErr := svcCtx.TaskManager.TryStart(taskID, id, svc.TaskTypeScheduledUpdate, func(taskCtx context.Context) {
		ctxWithTimeout, cancel := context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
		// 定时更新命中本程序自身时，同样走辅助容器方案，避免自己停自己卡死。
		if utiles.IsSelfContainer(svcCtx, id) {
			e := utiles.SelfUpdate(ctxWithTimeout, svcCtx, id, name, image, delOld, taskID, registryAuth)
			done <- e == nil
			return
		}
		e := utiles.UpdateContainerWithAuth(ctxWithTimeout, svcCtx, id, name, image, delOld, taskID, registryAuth)
		done <- e == nil
	})
	if startErr != nil {
		logx.Errorf("定时更新容器 %s 提交失败: %v", name, startErr)
		return false
	}
	return <-done
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

// pruneMissingContainers 过滤掉规则中已不存在的容器名，并在有变化时持久化。
// 返回过滤后的容器名列表，供本次执行直接使用。
func pruneMissingContainers(svcCtx *svc.ServiceContext, rule appconfig.ScheduledUpdateRule, existing map[string]struct{}) []string {
	kept := make([]string, 0, len(rule.ContainerNames))
	var removed []string
	for _, n := range rule.ContainerNames {
		if _, ok := existing[n]; ok {
			kept = append(kept, n)
		} else {
			removed = append(removed, n)
		}
	}
	if len(removed) == 0 {
		return kept
	}
	logx.Infof("定时更新规则[%s]自动移除已不存在的容器: %s", rule.Name, strings.Join(removed, ", "))
	// 持久化：把 kept 写回该规则的 ContainerNames
	_ = svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		for i := range cfg.ScheduledUpdates {
			if cfg.ScheduledUpdates[i].ID == rule.ID {
				cfg.ScheduledUpdates[i].ContainerNames = kept
				break
			}
		}
		return nil
	})
	return kept
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
