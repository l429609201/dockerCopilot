package scheduler

import (
	"sync"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/notify"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"
)

// Scheduler 管理所有定时更新规则的 cron 调度。
// 配置变更时通过 Reload 重建，保证运行中的调度始终与持久化配置一致。
type Scheduler struct {
	mu       sync.Mutex
	svcCtx   *svc.ServiceContext
	notifier notify.Notifier
	cron     *cron.Cron
	// notifyStop 用于停止固定周期的更新检测通知 goroutine。
	notifyStop chan struct{}
}

// New 创建调度器。notifier 可为 nil（此时不发送通知）。
func New(svcCtx *svc.ServiceContext, notifier notify.Notifier) *Scheduler {
	return &Scheduler{svcCtx: svcCtx, notifier: notifier}
}

// SetNotifier 更新通知渠道（如 Telegram 启用后注入）。
func (s *Scheduler) SetNotifier(n notify.Notifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = n
}

// Start 首次加载配置并启动调度，同时启动固定周期的更新检测通知器。
func (s *Scheduler) Start() {
	s.Reload()
	s.startUpdateNotifier()
}

// Reload 根据最新配置重建所有 cron 任务。
// 每个启用的规则使用自己的 cron 表达式独立调度。
func (s *Scheduler) Reload() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil {
		s.cron.Stop()
	}
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
	)))

	cfg := s.svcCtx.AppConfig.Get()
	addedCount := 0

	for _, rule := range cfg.ScheduledUpdates {
		if !rule.Enabled {
			continue
		}
		if rule.Cron == "" {
			logx.Infof("定时任务 [%s] 已跳过：未配置 cron 表达式", rule.Name)
			continue
		}

		// 解析简化配置或标准 cron 表达式
		cronExpr := appconfig.ParseCronExpression(rule.Cron)
		if cronExpr == "" {
			logx.Errorf("定时任务 [%s] 已跳过：cron 表达式无效 (%s)", rule.Name, rule.Cron)
			continue
		}

		// 闭包捕获当前规则（避免循环变量问题）
		r := rule
		_, err := c.AddFunc(cronExpr, func() {
			RunRule(s.svcCtx, s.notifier, r)
		})
		if err != nil {
			logx.Errorf("定时任务 [%s] 注册失败: %v (cron=%s)", rule.Name, err, cronExpr)
			continue
		}
		addedCount++
		logx.Infof("定时任务 [%s] 已注册: %s (原配置: %s)", rule.Name, cronExpr, rule.Cron)
	}

	c.Start()
	s.cron = c
	if addedCount > 0 {
		logx.Infof("定时任务调度器已启动，成功注册 %d 个任务", addedCount)
	} else {
		logx.Info("定时任务调度器：无有效任务")
	}
}

// RunNow 立即异步执行一条规则（用于手动触发）。
func (s *Scheduler) RunNow(rule appconfig.ScheduledUpdateRule) {
	go RunRule(s.svcCtx, s.notifier, rule)
}

// RunNowByID 按规则ID查找并立即执行，返回规则是否存在。实现 svc.Reloader 接口。
func (s *Scheduler) RunNowByID(ruleID string) bool {
	rule, ok := s.svcCtx.AppConfig.FindScheduledRule(ruleID)
	if !ok {
		return false
	}
	go RunRule(s.svcCtx, s.notifier, rule)
	return true
}

// Stop 停止调度与更新检测通知器。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
	}
	if s.notifyStop != nil {
		close(s.notifyStop)
		s.notifyStop = nil
	}
}
