package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/notify"
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

	// 获取所有主机的镜像列表并触发更新检查（覆盖远程主机，CheckUpdate 内部去重防并发）
	images, err := utiles.GetAllImagesList(s.svcCtx)
	if err != nil {
		logx.Errorf("更新检测获取镜像列表失败: %v", err)
		return
	}
	s.svcCtx.HubImageInfo.CheckUpdate(images)

	// 标记容器更新状态（聚合所有主机的容器，保证远程容器也能标记）
	containers, err := utiles.GetAllContainers(s.svcCtx)
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

	// 收集"有更新且未屏蔽"的容器（恢复屏蔽过滤，与详情页分离）
	var pending []notify.UpdateItem
	var mutedPending []notify.UpdateItem // 已屏蔽但有更新的容器
	for _, c := range containers {
		if !c.Update {
			continue
		}
		name := containerName(c)
		if name == "" {
			continue
		}
		// 优先使用 CreateImage，避免镜像更新后 Image 字段变空或变成 SHA256
		imageToUse := c.CreateImage
		if imageToUse == "" {
			imageToUse = c.Image // 降级使用 Image 字段
		}
		item := notify.UpdateItem{ID: c.ID, Name: name, Image: imageToUse}
		if _, ok := muted[name]; ok {
			mutedPending = append(mutedPending, item) // 已屏蔽
		} else {
			pending = append(pending, item) // 未屏蔽
		}
	}

	if len(pending) == 0 && len(mutedPending) == 0 {
		return
	}

	// 优先调用带交互式键盘的通知（Telegram Bot 实现了 notify.UpdateNotifierWithMuted）
	if kbNotifier, ok := s.notifier.(notify.UpdateNotifierWithMuted); ok {
		kbNotifier.NotifyUpdateWithMutedInfo(pending, mutedPending)
		return
	}
	// 降级：调用普通的带键盘通知（仅传未屏蔽的）
	if kbNotifier, ok := s.notifier.(notify.UpdateNotifier); ok {
		kbNotifier.NotifyUpdateWithKeyboard(pending)
		return
	}
	// 回退到普通文本通知（仅通知未屏蔽的）
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("检测到 %d 个容器有可用更新：\n\n", len(pending)))
	for _, c := range pending {
		msg.WriteString(fmt.Sprintf("🔺 %s\n   %s\n", c.Name, shortImage(c.Image)))
	}
	if len(mutedPending) > 0 {
		msg.WriteString(fmt.Sprintf("\n（另有 %d 个已屏蔽容器有更新）", len(mutedPending)))
	}
	msg.WriteString("\n💡 可在面板或发送 /update_all 进行更新")
	if s.notifier != nil {
		s.notifier.Notify("🔔 容器更新提醒", msg.String())
	}
}
