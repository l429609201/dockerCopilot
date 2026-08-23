package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/onlyLTY/dockerCopilot/internal/module/notify"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	MyType "github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
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
	registryAuth := ""
	if rule.RegistryID != "" {
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
			continue
		}
		// 策略：跳过无 tag 或 digest 形式镜像
		if rule.SkipInvalidTag && !isValidImageRef(c.Image) {
			logx.Infof("定时更新规则[%s]跳过非法镜像标签容器 %s (%s)", rule.Name, name, c.Image)
			skipped++
			continue
		}
		if runOne(svcCtx, c.ID, name, c.Image, !rule.KeepOldContainer, registryAuth, timeoutSec) {
			updated++
		} else {
			failed++
		}
	}

	summary := fmt.Sprintf("更新 %d，跳过 %d，失败 %d", updated, skipped, failed)
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时更新规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil && rule.NotifyOnDone {
		notifier.Notify("定时更新完成", fmt.Sprintf("规则「%s」：%s", rule.Name, summary))
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
