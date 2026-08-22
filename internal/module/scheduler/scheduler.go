package scheduler

import (
	"sync"

	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/onlyLTY/dockerCopilot/internal/module/notify"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
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

// Start 首次加载配置并启动调度。
func (s *Scheduler) Start() {
	s.Reload()
}

// Reload 根据最新配置重建所有 cron 任务。
// 每次配置增删改后调用，做到"配置即真相"。
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
	for _, rule := range cfg.ScheduledUpdates {
		if !rule.Enabled || rule.Cron == "" {
			continue
		}
		r := rule // 捕获副本，避免闭包共享循环变量
		_, err := c.AddFunc(r.Cron, func() {
			RunRule(s.svcCtx, s.notifier, r)
		})
		if err != nil {
			logx.Errorf("定时更新规则[%s] cron 表达式无效(%s): %v", r.Name, r.Cron, err)
		}
	}
	c.Start()
	s.cron = c
	logx.Infof("定时更新调度已重载，规则数：%d", len(cfg.ScheduledUpdates))
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

// Stop 停止调度。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
	}
}
