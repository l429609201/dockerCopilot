package bot

import (
	"fmt"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/zeromicro/go-zero/core/logx"
)

// buildPager 生成通用分页键盘：一行「上一页/页码/下一页」+ 一行「返回」。
// prefix 为翻页回调前缀（如 "imgpg"，翻页回调为 prefix|<page>）；
// page 当前页(从0开始)；totalPages 总页数；backData 返回按钮的回调数据。
// 仅在多页时显示翻页行；首页无「上一页」、尾页无「下一页」，中间页码按钮回调当前页(无动作占位)。
func buildPager(prefix string, page, totalPages int, backData string) [][]telegram.InlineKeyboardButton {
	var rows [][]telegram.InlineKeyboardButton
	if totalPages > 1 {
		var nav []telegram.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, telegram.InlineKeyboardButton{
				Text: "⬅ 上一页", CallbackData: fmt.Sprintf("%s|%d", prefix, page-1)})
		}
		// 页码指示按钮（点击无动作，回调当前页）
		nav = append(nav, telegram.InlineKeyboardButton{
			Text: fmt.Sprintf("%d/%d", page+1, totalPages), CallbackData: fmt.Sprintf("%s|%d", prefix, page)})
		if page < totalPages-1 {
			nav = append(nav, telegram.InlineKeyboardButton{
				Text: "下一页 ➡", CallbackData: fmt.Sprintf("%s|%d", prefix, page+1)})
		}
		rows = append(rows, nav)
	}
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: backData},
	})
	return rows
}

// reply 发送纯文本回复。
func (b *Bot) reply(chatID int64, text string) {
	if b.client == nil {
		return
	}
	if err := b.client.SendMessage(chatID, text, nil); err != nil {
		logx.Errorf("Telegram 回复失败 chat=%d: %v", chatID, err)
	}
}

// replyKeyboard 发送带 inline keyboard 的回复。
func (b *Bot) replyKeyboard(chatID int64, text string, kb *telegram.InlineKeyboardMarkup) {
	if b.client == nil {
		return
	}
	if err := b.client.SendMessage(chatID, text, kb); err != nil {
		logx.Errorf("Telegram 回复失败 chat=%d: %v", chatID, err)
	}
}

// escapeHTML 转义 HTML 特殊字符，避免注入破坏消息格式。
func escapeHTML(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(s)
}

// deleteMsg 删除指定消息（失败仅记录日志，不影响主流程）。
// 用于交互式 Shell 会话：执行后删除用户发送的命令消息，保持对话干净。
func (b *Bot) deleteMsg(chatID, messageID int64) {
	if b.client == nil || messageID == 0 {
		return
	}
	if err := b.client.DeleteMessage(chatID, messageID); err != nil {
		logx.Errorf("Telegram 删除消息失败 chat=%d msg=%d: %v", chatID, messageID, err)
	}
}

// helpText 返回帮助文案。
// 现版本以「按钮式菜单」为主，帮助内容对齐主菜单的 8 个入口，指导用户点击操作，
// 而非记忆命令（命令仍可用，作为补充在末尾列出）。
func helpText() string {
	return "<b>DockerCopilot 使用帮助</b>\n\n" +
		"发送 /menu 或 /start 打开主菜单，全部操作都能点按钮完成：\n\n" +
		"📊 <b>概览</b> — 系统统计：容器/镜像数量、磁盘占用\n" +
		"📦 <b>容器</b> — 容器列表，点「⚙ 管理」进入单容器面板\n" +
		"🆙 <b>更新</b> — 检查镜像更新、一键批量更新\n" +
		"🐳 <b>镜像</b> — 查看镜像列表（名称、大小、是否在用）\n" +
		"📋 <b>备份</b> — 备份与恢复容器配置\n" +
		"💻 <b>实例</b> — 管理多个 Docker 实例\n" +
		"⚙️ <b>设置</b> — 通知、更新策略等配置\n" +
		"📚 <b>帮助</b> — 显示本页说明\n\n" +
		"<b>单容器面板</b>（在容器列表点「⚙ 管理」）可用：\n" +
		"启动/停止/重启/暂停/恢复、更新、切换标签、查看日志、\n" +
		"详情、资源、命令行（Shell）、重命名、删除等。\n\n" +
		"<b>命令行（Shell）</b>：在面板点「💻 命令行」后，直接发送命令\n" +
		"即可连续执行，<code>cd</code> 会保持工作目录；点「⏹ 退出交互」\n" +
		"返回容器面板。\n\n" +
		"<b>快捷命令（可选）</b>：/ps 容器列表、/images 镜像、\n" +
		"/sys 概览、/check_updates 检查更新、/compose 管理 Compose 项目。"
}

// stateLabel 将 Docker 容器状态汉化并加上 emoji 图标。
func stateLabel(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return "🟢运行中"
	case "exited":
		return "⚪已停止"
	case "paused":
		return "⏸已暂停"
	case "restarting":
		return "🔄重启中"
	case "created":
		return "🆕已创建"
	case "removing":
		return "🗑清理中"
	case "dead":
		return "💀异常"
	default:
		return state
	}
}

// shortImage 精简镜像名用于展示：去掉 registry 前缀，仅保留 名称:标签。
// 例：ghcr.io/l429609201/dd-danmaku:test -> dd-danmaku:test
func shortImage(image string) string {
	if image == "" {
		return "-"
	}
	// 去掉可能的 sha256 摘要形式
	if strings.HasPrefix(image, "sha256:") {
		if len(image) > 19 {
			return image[7:19]
		}
		return image
	}
	// 取最后一段路径（去掉 registry / namespace）
	seg := image
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		seg = image[idx+1:]
	}
	return seg
}

// actionLabel 返回操作的中文标签。
func actionLabel(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	case "pause":
		return "暂停"
	case "unpause":
		return "恢复"
	case "kill":
		return "强制终止"
	case "remove":
		return "删除"
	case "update":
		return "更新"
	default:
		return action
	}
}

// actionUsage 返回操作的用法提示。
func actionUsage(action string) string {
	switch action {
	case "start":
		return "/start_c <容器名>"
	case "stop":
		return "/stop_c <容器名>"
	case "restart":
		return "/restart_c <容器名>"
	default:
		return "/" + action + " <容器名>"
	}
}
