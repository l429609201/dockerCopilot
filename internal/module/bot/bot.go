package bot

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/onlyLTY/dockerCopilot/internal/module/containerops"
	"github.com/onlyLTY/dockerCopilot/internal/module/telegram"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// Bot 是 Telegram 机器人服务：长轮询接收指令、执行容器操作、作为通知渠道。
// 实现 notify.Notifier 接口，可注入 scheduler 用于定时更新结果推送。
type Bot struct {
	svcCtx  *svc.ServiceContext
	ops     *containerops.Service
	mu      sync.Mutex
	client  *telegram.Client
	cfg     appconfig.TelegramConfig
	running atomic.Bool
	stopCh  chan struct{}
}

// New 创建 Bot 服务实例（未启动）。
func New(svcCtx *svc.ServiceContext) *Bot {
	return &Bot{
		svcCtx: svcCtx,
		ops:    containerops.New(svcCtx),
	}
}

// Notify 实现 notify.Notifier：向所有白名单会话推送通知。
// 未启用或未配置白名单时静默忽略，不阻塞调用方。
func (b *Bot) Notify(title string, text string) {
	b.mu.Lock()
	client := b.client
	cfg := b.cfg
	b.mu.Unlock()
	if client == nil || !cfg.Enabled || !cfg.NotifyUpdate {
		return
	}
	msg := "<b>" + escapeHTML(title) + "</b>\n" + escapeHTML(text)
	for _, chatID := range cfg.AllowedChatIDs {
		if err := client.SendMessage(chatID, msg, nil); err != nil {
			logx.Errorf("Telegram 通知发送失败 chat=%d: %v", chatID, err)
		}
	}
}

// Reload 根据最新配置重建 Bot：停止旧轮询，按需启动新轮询。
func (b *Bot) Reload() {
	cfg := b.svcCtx.AppConfig.Get().Telegram
	b.Stop()
	if !cfg.Enabled || cfg.Token == "" {
		logx.Info("Telegram Bot 未启用或未配置 Token，跳过启动")
		return
	}
	client, err := telegram.NewClient(cfg.Token, cfg.Proxy)
	if err != nil {
		logx.Errorf("创建 Telegram 客户端失败: %v", err)
		return
	}
	b.mu.Lock()
	b.client = client
	b.cfg = cfg
	b.stopCh = make(chan struct{})
	b.mu.Unlock()

	b.running.Store(true)
	go b.pollLoop()
	logx.Info("Telegram Bot 已启动")
}

// Stop 停止轮询。
func (b *Bot) Stop() {
	if !b.running.CompareAndSwap(true, false) {
		return
	}
	b.mu.Lock()
	if b.stopCh != nil {
		close(b.stopCh)
		b.stopCh = nil
	}
	b.mu.Unlock()
}

// pollLoop 长轮询主循环，处理更新并对失败做退避。
func (b *Bot) pollLoop() {
	var offset int
	interval := b.cfg.PollIntervalSec
	if interval <= 0 {
		interval = 3
	}
	for b.running.Load() {
		b.mu.Lock()
		client := b.client
		stopCh := b.stopCh
		b.mu.Unlock()
		if client == nil {
			return
		}
		updates, err := client.GetUpdates(offset, 30)
		if err != nil {
			logx.Errorf("Telegram getUpdates 失败: %v", err)
			select {
			case <-stopCh:
				return
			case <-time.After(time.Duration(interval) * time.Second):
				continue
			}
		}
		for _, u := range updates {
			offset = int(u.UpdateID) + 1
			b.handleUpdate(u)
		}
	}
}

// isAllowed 判断会话是否在白名单内。
func (b *Bot) isAllowed(chatID int64) bool {
	for _, id := range b.cfg.AllowedChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}
