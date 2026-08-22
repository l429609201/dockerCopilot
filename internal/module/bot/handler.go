package bot

import (
	"fmt"
	"strings"

	"github.com/onlyLTY/dockerCopilot/internal/module/telegram"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// handleUpdate 分发消息与回调，统一做白名单鉴权。
func (b *Bot) handleUpdate(u telegram.Update) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("Telegram 处理更新 panic 已恢复: %v", r)
		}
	}()
	chatID := u.ChatID()
	if chatID == 0 {
		return
	}
	if !b.isAllowed(chatID) {
		// 非白名单用户：明确拒绝，不泄露任何容器信息
		_ = b.client.SendMessage(chatID, "⛔ 未授权：你的 Chat ID 不在白名单内。", nil)
		logx.Errorf("Telegram 拒绝未授权会话: %d", chatID)
		return
	}
	if u.CallbackQuery != nil {
		b.handleCallback(chatID, u.CallbackQuery)
		return
	}
	if u.Message != nil {
		b.handleCommand(chatID, strings.TrimSpace(u.Message.Text))
	}
}

// handleCommand 处理文本指令。
func (b *Bot) handleCommand(chatID int64, text string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	cmd := fields[0]
	args := fields[1:]
	switch cmd {
	case "/start", "/help":
		b.reply(chatID, helpText())
	case "/ps", "/containers":
		b.replyContainerList(chatID)
	case "/images":
		b.replyImageList(chatID)
	case "/start_c":
		b.doAction(chatID, args, "start")
	case "/stop_c":
		b.doAction(chatID, args, "stop")
	case "/restart_c":
		b.doAction(chatID, args, "restart")
	default:
		b.reply(chatID, "未知指令，发送 /help 查看支持的指令。")
	}
}

// doAction 对指定容器名执行低风险操作（启动/停止/重启），带二次确认。
func (b *Bot) doAction(chatID int64, args []string, action string) {
	if len(args) == 0 {
		b.reply(chatID, "用法："+actionUsage(action))
		return
	}
	name := args[0]
	id, err := b.ops.ResolveIDByName(name)
	if err != nil {
		b.reply(chatID, "❌ "+err.Error())
		return
	}
	// 通过 inline keyboard 二次确认，callback data 形如 "confirm|action|id|name"
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "✅ 确认" + actionLabel(action), CallbackData: fmt.Sprintf("confirm|%s|%s|%s", action, id, name)},
			{Text: "取消", CallbackData: "cancel"},
		}},
	}
	b.replyKeyboard(chatID, fmt.Sprintf("确认对容器 <b>%s</b> 执行「%s」？", escapeHTML(name), actionLabel(action)), kb)
}

// handleCallback 处理 inline 按钮回调，执行已确认的操作。
func (b *Bot) handleCallback(chatID int64, cb *telegram.CallbackQuery) {
	_ = b.client.AnswerCallbackQuery(cb.ID, "")
	if cb.Data == "cancel" {
		b.reply(chatID, "已取消。")
		return
	}
	parts := strings.Split(cb.Data, "|")
	if len(parts) != 4 || parts[0] != "confirm" {
		return
	}
	action, id, name := parts[1], parts[2], parts[3]
	var err error
	switch action {
	case "start":
		err = b.ops.Start(id)
	case "stop":
		err = b.ops.Stop(id, 10)
	case "restart":
		err = b.ops.Restart(id, 10)
	default:
		b.reply(chatID, "不支持的操作")
		return
	}
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 容器 %s 执行「%s」失败：%s", name, actionLabel(action), err.Error()))
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ 容器 %s 已%s", name, actionLabel(action)))
}

// replyContainerList 推送容器列表概览。
func (b *Bot) replyContainerList(chatID int64) {
	list, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取容器列表失败："+err.Error())
		return
	}
	if len(list) == 0 {
		b.reply(chatID, "当前没有容器。")
		return
	}
	var sb strings.Builder
	sb.WriteString("<b>容器列表</b>\n")
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		sb.WriteString(fmt.Sprintf("• %s [%s]\n", escapeHTML(name), escapeHTML(c.State)))
	}
	b.reply(chatID, sb.String())
}

// replyImageList 推送镜像数量概览。
func (b *Bot) replyImageList(chatID int64) {
	list, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取镜像列表失败："+err.Error())
		return
	}
	b.reply(chatID, fmt.Sprintf("当前镜像数量：%d", len(list)))
}
