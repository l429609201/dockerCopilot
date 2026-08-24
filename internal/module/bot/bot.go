package bot

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/notify"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/svc"
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
	// pending 记录每个会话待完成的输入型动作（重命名/切标签等一次性动作），
	// 用户点击按钮后进入等待，下一条文本消息作为输入完成动作。
	pendingMu sync.Mutex
	pending   map[int64]*pendingAction
	// shells 记录每个会话进入的"交互式 Shell 会话"（持续，直到 /exit）。
	// 与 pending 区分：pending 是一次性动作，shells 是连续命令会话，需保持工作目录。
	shellMu sync.Mutex
	shells  map[int64]*shellSession
}

// pendingAction 描述一个等待用户文本输入的动作。
type pendingAction struct {
	kind string // rename / tag
	id   string // 目标容器ID
	name string // 目标容器名
}

// shellSession 描述一个持续的容器 Shell 会话。
// 进入后用户连续发送的每条文本都会作为命令在该容器内执行，直到 /exit 退出。
// workDir 记录当前工作目录，使连续的 cd 生效（通过每次命令末尾回写 pwd 实现）。
type shellSession struct {
	id          string   // 目标容器ID
	name        string   // 目标容器名
	workDir     string   // 当前工作目录（空表示容器默认目录）
	resultMsgID int64    // "终端消息"ID：结果始终更新在这一条上
	history     []string // 已执行命令历史（用于"查看历史命令"）
}

// New 创建 Bot 服务实例（未启动）。
func New(svcCtx *svc.ServiceContext) *Bot {
	return &Bot{
		svcCtx:  svcCtx,
		ops:     containerops.New(svcCtx),
		pending: make(map[int64]*pendingAction),
		shells:  make(map[int64]*shellSession),
	}
}

// setPending 登记会话的待输入动作。
func (b *Bot) setPending(chatID int64, p *pendingAction) {
	b.pendingMu.Lock()
	b.pending[chatID] = p
	b.pendingMu.Unlock()
}

// takePending 取出并清除会话的待输入动作。
func (b *Bot) takePending(chatID int64) *pendingAction {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	p := b.pending[chatID]
	delete(b.pending, chatID)
	return p
}

// getShell 返回会话当前的 Shell 会话（不存在则返回 nil）。
func (b *Bot) getShell(chatID int64) *shellSession {
	b.shellMu.Lock()
	defer b.shellMu.Unlock()
	return b.shells[chatID]
}

// setShell 进入/更新一个 Shell 会话。
func (b *Bot) setShell(chatID int64, s *shellSession) {
	b.shellMu.Lock()
	b.shells[chatID] = s
	b.shellMu.Unlock()
}

// clearShell 退出并清除 Shell 会话。
func (b *Bot) clearShell(chatID int64) {
	b.shellMu.Lock()
	delete(b.shells, chatID)
	b.shellMu.Unlock()
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

// NotifyUpdateWithKeyboard 推送带交互式键盘的更新通知（每个容器一行操作按钮）。
// containers 为需要更新的容器列表；参数类型为 notify.UpdateItem，使本方法满足
// notify.UpdateNotifier 接口，让 scheduler 的周期检测能命中带键盘的推送而非纯文本。
func (b *Bot) NotifyUpdateWithKeyboard(containers []UpdateContainer) {
	b.mu.Lock()
	client := b.client
	cfg := b.cfg
	b.mu.Unlock()
	if client == nil || !cfg.Enabled || !cfg.NotifyUpdate {
		return
	}

	for _, chatID := range cfg.AllowedChatIDs {
		b.sendUpdateNotificationToChat(chatID, containers)
	}
}

// UpdateContainer 是 notify.UpdateItem 的类型别名，供 Bot 内部复用同一结构，
// 保证 NotifyUpdateWithKeyboard 的签名与 notify.UpdateNotifier 接口一致。
type UpdateContainer = notify.UpdateItem

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
	// 注册命令菜单，让 TG 客户端输入框旁展示可用命令
	if err := client.SetMyCommands(botCommands()); err != nil {
		logx.Errorf("Telegram 设置命令菜单失败: %v", err)
	}
	go b.pollLoop()
	logx.Info("Telegram Bot 已启动")
}

// botCommands 返回注册到 Telegram 客户端的命令菜单列表。
func botCommands() []telegram.BotCommand {
	return []telegram.BotCommand{
		{Command: "start", Description: "打开主菜单"},
		{Command: "ps", Description: "查看容器列表"},
		{Command: "sys", Description: "系统概览"},
		{Command: "check_updates", Description: "检查所有更新"},
		{Command: "update_all", Description: "更新所有容器"},
		{Command: "images", Description: "查看镜像列表"},
		{Command: "compose", Description: "查看 Compose 项目"},
		{Command: "help", Description: "查看帮助"},
	}
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
