package scheduler

import (
	"context"
	"fmt"
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

	images, err := utiles.GetImagesList(svcCtx)
	if err != nil {
		logx.Errorf("定时清理规则[%s]获取镜像列表失败: %v", rule.Name, err)
		if notifier != nil && rule.NotifyOnError {
			notifier.Notify("定时清理失败", fmt.Sprintf("规则「%s」获取镜像列表失败：%s", rule.Name, err.Error()))
		}
		recordResult(svcCtx, rule.ID, "获取镜像列表失败："+err.Error())
		return
	}

	// 按范围筛选待清理镜像 ID
	mode := rule.PruneMode
	if mode == "" {
		mode = appconfig.PruneModeDangling
	}
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
		summary := "没有需要清理的镜像"
		recordResult(svcCtx, rule.ID, summary)
		if notifier != nil && rule.NotifyOnDone {
			notifier.Notify("定时清理完成", fmt.Sprintf("规则「%s」执行完成：%s", rule.Name, summary))
		}
		return
	}

	// 提交批量清理任务并等待完成
	taskID := uuid.New().String()
	done := make(chan struct{}, 1)
	startErr := svcCtx.TaskManager.TryStart(taskID, "prune-"+rule.ID, svc.TaskTypeImagePrune, func(taskCtx context.Context) {
		utiles.PruneImages(taskCtx, svcCtx, taskID, ids, false)
		done <- struct{}{}
	})
	if startErr != nil {
		logx.Errorf("定时清理规则[%s]提交任务失败: %v", rule.Name, startErr)
		recordResult(svcCtx, rule.ID, "提交清理任务失败："+startErr.Error())
		if notifier != nil && rule.NotifyOnError {
			notifier.Notify("定时清理失败", fmt.Sprintf("规则「%s」提交清理任务失败：%s", rule.Name, startErr.Error()))
		}
		return
	}
	<-done

	summary := fmt.Sprintf("已提交清理 %d 个镜像（范围：%s）", len(ids), pruneModeLabel(mode))
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时清理规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil && rule.NotifyOnDone {
		notifier.Notify("定时清理完成", fmt.Sprintf("规则「%s」执行完成\n\n🧹 %s", rule.Name, summary))
	}
}

// runBackup 执行"自动备份"规则：全量备份所有容器配置为 JSON 文件。
// 底层 utiles.BackupContainer 备份全部容器，故本类型忽略容器选择。
func runBackup(svcCtx *svc.ServiceContext, notifier notify.Notifier, rule appconfig.ScheduledUpdateRule) {
	if notifier != nil && rule.NotifyOnStart {
		notifier.Notify("定时备份开始", fmt.Sprintf("规则「%s」开始备份容器配置", rule.Name))
	}

	err := utiles.BackupContainer(svcCtx)
	if err != nil {
		logx.Errorf("定时备份规则[%s]执行失败: %v", rule.Name, err)
		recordResult(svcCtx, rule.ID, "备份失败："+err.Error())
		if notifier != nil && rule.NotifyOnError {
			notifier.Notify("定时备份失败", fmt.Sprintf("规则「%s」备份失败：%s", rule.Name, err.Error()))
		}
		return
	}

	summary := fmt.Sprintf("备份成功（%s）", time.Now().Format("2006-01-02 15:04:05"))
	recordResult(svcCtx, rule.ID, summary)
	logx.Infof("定时备份规则[%s]执行完成：%s", rule.Name, summary)
	if notifier != nil && rule.NotifyOnDone {
		notifier.Notify("定时备份完成", fmt.Sprintf("规则「%s」执行完成\n\n💾 %s", rule.Name, summary))
	}
}

// pruneModeLabel 返回清理范围的中文标签。
func pruneModeLabel(mode string) string {
	if mode == appconfig.PruneModeUnused {
		return "所有未使用镜像"
	}
	return "无tag悬空镜像"
}
