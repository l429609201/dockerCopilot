package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
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
	case "/start", "/menu":
		b.sendMainMenu(chatID)
	case "/help":
		b.reply(chatID, helpText())
	case "/ps", "/containers":
		b.replyContainerList(chatID, false, 0)
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
	// 复用统一的二次确认逻辑
	b.askConfirm(chatID, action, id, name)
}

// handleCallback 处理 inline 按钮回调：主菜单跳转、列表操作确认、执行已确认操作。
func (b *Bot) handleCallback(chatID int64, cb *telegram.CallbackQuery) {
	_ = b.client.AnswerCallbackQuery(cb.ID, "")
	if cb.Data == "cancel" {
		b.reply(chatID, "已取消。")
		return
	}
	parts := strings.Split(cb.Data, "|")
	// 主菜单按钮：menu|<目标>
	if parts[0] == "menu" && len(parts) >= 2 {
		page := 0
		if len(parts) == 3 {
			if n, e := strconv.Atoi(parts[2]); e == nil {
				page = n
			}
		}
		switch parts[1] {
		case "ps":
			b.replyContainerList(chatID, false, page)
		case "run":
			b.replyContainerList(chatID, true, page)
		case "images":
			b.replyImageList(chatID)
		case "help":
			b.reply(chatID, helpText())
		}
		return
	}
	// 列表操作按钮：act|<action>|<id>|<name>，弹出二次确认
	if len(parts) == 4 && parts[0] == "act" {
		b.askConfirm(chatID, parts[1], parts[2], parts[3])
		return
	}
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

// sendMainMenu 推送主菜单（按钮式交互入口）。
func (b *Bot) sendMainMenu(chatID int64) {
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "▶ 运行中容器", CallbackData: "menu|run|0"}},
			{{Text: "📦 全部容器", CallbackData: "menu|ps|0"}},
			{{Text: "🖼 镜像信息", CallbackData: "menu|images"}, {Text: "❓ 帮助", CallbackData: "menu|help"}},
		},
	}
	b.replyKeyboard(chatID, "<b>DockerCopilot 控制台</b>\n请选择操作：", kb)
}

// askConfirm 对指定容器操作弹出二次确认按钮。
func (b *Bot) askConfirm(chatID int64, action, id, name string) {
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "✅ 确认" + actionLabel(action), CallbackData: fmt.Sprintf("confirm|%s|%s|%s", action, id, name)},
			{Text: "取消", CallbackData: "cancel"},
		}},
	}
	b.replyKeyboard(chatID, fmt.Sprintf("确认对容器 <b>%s</b> 执行「%s」？", escapeHTML(name), actionLabel(action)), kb)
}

// pageSize 每页展示的容器数量。
const pageSize = 8

// replyContainerList 分页推送容器列表，每个容器附带操作按钮。
// onlyRunning=true 时仅展示运行中容器；page 从 0 开始。
func (b *Bot) replyContainerList(chatID int64, onlyRunning bool, page int) {
	list, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取容器列表失败："+err.Error())
		return
	}
	// 过滤
	filtered := list[:0:0]
	for _, c := range list {
		if onlyRunning && !strings.EqualFold(c.State, "running") {
			continue
		}
		filtered = append(filtered, c)
	}
	if len(filtered) == 0 {
		b.reply(chatID, "没有符合条件的容器。")
		return
	}
	// 分页边界
	if page < 0 {
		page = 0
	}
	total := len(filtered)
	totalPages := (total + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	title := "全部容器"
	if onlyRunning {
		title = "运行中容器"
	}
	b.reply(chatID, fmt.Sprintf("<b>%s</b>（第 %d/%d 页，共 %d 个，点按钮操作）", title, page+1, totalPages, total))
	for _, c := range filtered[start:end] {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		var row []telegram.InlineKeyboardButton
		if strings.EqualFold(c.State, "running") {
			row = append(row,
				telegram.InlineKeyboardButton{Text: "⏹ 停止", CallbackData: fmt.Sprintf("act|stop|%s|%s", id, name)},
				telegram.InlineKeyboardButton{Text: "🔄 重启", CallbackData: fmt.Sprintf("act|restart|%s|%s", id, name)},
			)
		} else {
			row = append(row,
				telegram.InlineKeyboardButton{Text: "▶ 启动", CallbackData: fmt.Sprintf("act|start|%s|%s", id, name)},
			)
		}
		kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{row}}
		b.replyKeyboard(chatID, fmt.Sprintf("• <b>%s</b> [%s]", escapeHTML(name), escapeHTML(c.State)), kb)
	}

	// 翻页按钮
	mode := "ps"
	if onlyRunning {
		mode = "run"
	}
	var nav []telegram.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("menu|%s|%d", mode, page-1)})
	}
	if page < totalPages-1 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("menu|%s|%d", mode, page+1)})
	}
	if len(nav) > 0 {
		kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{nav}}
		b.replyKeyboard(chatID, fmt.Sprintf("— 第 %d/%d 页 —", page+1, totalPages), kb)
	}
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
