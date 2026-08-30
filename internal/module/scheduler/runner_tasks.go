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
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// runPrune 执行"自动清理镜像"规则：按清理范围筛选镜像 -> 提交批量清理任务。
//   - dangling：仅清理无 tag（<none>）的悬空镜像
//   - unused：清理所有未被容器使用的镜像
func runPrune(svcCtx *svc.ServiceContext, notifier notify.Notifier, rule appconfig.ScheduledUpdateRule) {
	if notifier != nil && rule.NotifyOnStart {
		notifier.Notify("定时清理开始", fmt.Sprintf("规则「%s」开始清理镜像（范围：%s）", rule.Name, pruneModeLabel(rule.PruneMode)))
	}

	mode := rule.PruneMode
	if mode == "" {
		mode = appconfig.PruneModeDangling
	}

	// 目标主机：规则指定则用指定，否则仅本地
	hosts := effectiveHostIDs(rule)
	var totalPruned int
	var perHostSummary []string
	var anyErr bool

	for _, hostID := range hosts {
		hostName := hostDisplayName(svcCtx, hostID)
		// 拉取该主机镜像列表
		images, err := utiles.GetImagesListFromHost(svcCtx, hostID)
		if err != nil {
			anyErr = true
			logx.Errorf("定时清理规则[%s]获取主机[%s]镜像列表失败: %v", rule.Name, hostName, err)
			perHostSummary = append(perHostSummary, fmt.Sprintf("%s：获取镜像失败(%s)", hostName, err.Error()))
			continue
		}
		// 按范围筛选待清理镜像 ID
		var ids []string
		for _, img := range images {
			switch mode {
			case appconfig.PruneModeUnused:
				if !img.InUsed {
					ids = append(ids, img.ID)
				}
			default: // dangling
				if img.ImageName == "None" || img.ImageTag == "None" {
					ids = append(ids, img.ID)
				}
			}
		}
		if len(ids) == 0 {
			perHostSummary = append(perHostSummary, fmt.Sprintf("%s：无需清理", hostName))
			continue
		}
		// 在该主机提交批量清理任务并等待完成
		taskID := uuid.New().String()
		done := make(chan struct{}, 1)
		startErr := svcCtx.TaskManager.TryStart(taskID, "prune-"+rule.ID+"-"+hostID, svc.TaskTypeImagePrune, func(taskCtx context.Context) {
			utiles.PruneImagesOnHost(taskCtx, svcCtx, hostID, taskID, ids, false)
			done <- struct{}{}
		})
		if startErr != nil {
			anyErr = true
			logx.Errorf("定时清理规则[%s]主机[%s]提交任务失败: %v", rule.Name, hostName, startErr)
			perHostSummary = append(perHostSummary, fmt.Sprintf("%s：提交失败(%s)", hostName, startErr.Error()))
			continue
		}
		<-done
		totalPruned += len(ids)
		perHostSummary = append(perHostSummary, fmt.Sprintf("%s：清理 %d 个", hostName, len(ids)))
	}

	summary := fmt.Sprintf("共清理 %d 个镜像（范围：%s）", totalPruned, pruneModeLabel(mode))
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时清理规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil {
		if anyErr && rule.NotifyOnError {
			notifier.Notify("定时清理部分失败", fmt.Sprintf("规则「%s」执行完成（含失败）\n\n%s", rule.Name, strings.Join(perHostSummary, "\n")))
		} else if rule.NotifyOnDone {
			notifier.Notify("定时清理完成", fmt.Sprintf("规则「%s」执行完成\n\n🧹 %s\n%s", rule.Name, summary, strings.Join(perHostSummary, "\n")))
		}
	}
}

// runBackup 执行"自动备份"规则：全量备份所有容器配置为 JSON 文件。
// 底层 utiles.BackupContainer 备份全部容器，故本类型忽略容器选择。
func runBackup(svcCtx *svc.ServiceContext, notifier notify.Notifier, rule appconfig.ScheduledUpdateRule) {
	if notifier != nil && rule.NotifyOnStart {
		notifier.Notify("定时备份开始", fmt.Sprintf("规则「%s」开始备份容器配置", rule.Name))
	}

	// 目标主机：规则指定则用指定，否则仅本地。逐主机备份为独立文件。
	hosts := effectiveHostIDs(rule)
	var okCount int
	var perHostSummary []string
	var anyErr bool
	for _, hostID := range hosts {
		hostName := hostDisplayName(svcCtx, hostID)
		if err := utiles.BackupContainerOnHost(svcCtx, hostID); err != nil {
			anyErr = true
			logx.Errorf("定时备份规则[%s]主机[%s]失败: %v", rule.Name, hostName, err)
			perHostSummary = append(perHostSummary, fmt.Sprintf("%s：失败(%s)", hostName, err.Error()))
			continue
		}
		okCount++
		hostMsg := fmt.Sprintf("%s：成功", hostName)
		// 备份成功后，按规则的最大保留数清理该主机的旧备份（0/负数=不限制）。
		// 清理失败不影响备份本身，仅记录日志与摘要。
		if rule.MaxBackups > 0 {
			if deleted, cErr := utiles.CleanupBackupsForHost(hostID, rule.MaxBackups); cErr != nil {
				logx.Errorf("定时备份规则[%s]主机[%s]清理旧备份失败: %v", rule.Name, hostName, cErr)
			} else if deleted > 0 {
				hostMsg = fmt.Sprintf("%s：成功（清理旧备份 %d 个，保留最近 %d 个）", hostName, deleted, rule.MaxBackups)
			}
		}
		perHostSummary = append(perHostSummary, hostMsg)
	}

	summary := fmt.Sprintf("备份完成 %d/%d 个主机（%s）", okCount, len(hosts), time.Now().Format("2006-01-02 15:04:05"))
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时备份规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil {
		if anyErr && rule.NotifyOnError {
			notifier.Notify("定时备份部分失败", fmt.Sprintf("规则「%s」执行完成（含失败）\n\n%s", rule.Name, strings.Join(perHostSummary, "\n")))
		} else if rule.NotifyOnDone {
			notifier.Notify("定时备份完成", fmt.Sprintf("规则「%s」执行完成\n\n💾 %s\n%s", rule.Name, summary, strings.Join(perHostSummary, "\n")))
		}
	}
}

// effectiveHostIDs 解析规则的目标主机列表：为空时返回仅本地。
func effectiveHostIDs(rule appconfig.ScheduledUpdateRule) []string {
	if len(rule.HostIDs) == 0 {
		return []string{appconfig.DockerHostLocalID}
	}
	out := make([]string, 0, len(rule.HostIDs))
	for _, h := range rule.HostIDs {
		if h == "" {
			h = appconfig.DockerHostLocalID
		}
		out = append(out, h)
	}
	return out
}

// hostDisplayName 返回主机展示名，找不到时回退主机ID。
func hostDisplayName(svcCtx *svc.ServiceContext, hostID string) string {
	if host, ok := svcCtx.AppConfig.FindDockerHost(hostID); ok && host.Name != "" {
		return host.Name
	}
	return hostID
}

// pruneModeLabel 返回清理范围的中文标签。
func pruneModeLabel(mode string) string {
	if mode == appconfig.PruneModeUnused {
		return "所有未使用镜像"
	}
	return "无tag悬空镜像"
}
