package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// defaultCheckInterval 内置更新检测的默认周期（配置未设置或非法时使用）。
const defaultCheckInterval = 30 * time.Minute

// resolveCheckInterval 读取配置的检测周期（分钟），非法值回退默认 30 分钟。
func (s *Scheduler) resolveCheckInterval() time.Duration {
	m := s.svcCtx.AppConfig.Get().Telegram.UpdateCheckIntervalMinutes
	if m <= 0 {
		return defaultCheckInterval
	}
	return time.Duration(m) * time.Minute
}

// startUpdateNotifier 启动可配置周期的更新检测通知器。
// 用 Timer 循环，每轮结束后按最新配置重设下次间隔，周期变更下一轮即生效。
// 检测所有容器镜像更新，排除已屏蔽容器，去重后推送到通知渠道（如 Telegram）。
func (s *Scheduler) startUpdateNotifier() {
	s.mu.Lock()
	// 避免重复启动：Reload 不触碰此 goroutine，仅 Start 启动一次。
	if s.notifyStop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.notifyStop = stop
	s.mu.Unlock()

	go func() {
		timer := time.NewTimer(s.resolveCheckInterval())
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
				s.runUpdateCheck()
				// 按最新配置重设下次触发间隔，实现周期动态可调
				timer.Reset(s.resolveCheckInterval())
			}
		}
	}()
	logx.Infof("内置更新检测通知器已启动，初始周期：%s", s.resolveCheckInterval())
}

// runUpdateCheck 执行一次更新检测并推送。带 panic 兜底，单次失败不影响后续周期。
func (s *Scheduler) runUpdateCheck() {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("更新检测通知发生 panic 已恢复: %v", r)
		}
	}()

	cfg := s.svcCtx.AppConfig.Get()
	// 仅在 TG 启用且更新通知开关打开时才检测推送，避免无谓开销。
	if !cfg.Telegram.Enabled || !cfg.Telegram.NotifyUpdate {
		return
	}

	// 获取镜像列表并触发更新检查（CheckUpdate 内部去重防并发）
	images, err := utiles.GetImagesList(s.svcCtx)
	if err != nil {
		logx.Errorf("更新检测获取镜像列表失败: %v", err)
		return
	}
	s.svcCtx.HubImageInfo.CheckUpdate(images)

	// 标记容器更新状态
	containers, err := utiles.GetContainerList(s.svcCtx)
	if err != nil {
		logx.Errorf("更新检测获取容器列表失败: %v", err)
		return
	}
	containers = utiles.CheckImageUpdate(s.svcCtx, containers)

	// 屏蔽列表转 set
	muted := make(map[string]struct{}, len(cfg.Telegram.MutedContainers))
	for _, n := range cfg.Telegram.MutedContainers {
		muted[n] = struct{}{}
	}

	// 收集"有更新且未屏蔽且未通知过当前版本"的容器
	notified := cfg.NotifiedVersions
	if notified == nil {
		notified = map[string]string{}
	}
	var pending []MyContainer
	for _, c := range containers {
		if !c.Update {
			continue
		}
		name := containerName(c)
		if name == "" {
			continue
		}
		if _, ok := muted[name]; ok {
			continue // 已屏蔽
		}
		// 去重：同一容器同一镜像版本只推一次
		if last, ok := notified[name]; ok && last == c.Image {
			continue
		}
		pending = append(pending, MyContainer{Name: name, Image: c.Image})
	}

	if len(pending) == 0 {
		return
	}

	// 组织推送文案
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("检测到 %d 个容器有可用更新：\n\n", len(pending)))
	for _, c := range pending {
		msg.WriteString(fmt.Sprintf("🔺 %s\n   %s\n", c.Name, shortImage(c.Image)))
	}
	msg.WriteString("\n💡 可在面板或发送 /update_all 进行更新")
	if s.notifier != nil {
		s.notifier.Notify("🔔 容器更新提醒", msg.String())
	}

	// 记录已通知版本，持久化去重状态
	_ = s.svcCtx.AppConfig.Update(func(ac *appconfig.AppConfig) error {
		if ac.NotifiedVersions == nil {
			ac.NotifiedVersions = map[string]string{}
		}
		for _, c := range pending {
			ac.NotifiedVersions[c.Name] = c.Image
		}
		return nil
	})
}

// MyContainer 更新检测的轻量容器信息。
type MyContainer struct {
	Name  string
	Image string
}
