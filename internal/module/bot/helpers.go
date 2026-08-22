package bot

import (
	"strings"

	"github.com/onlyLTY/dockerCopilot/internal/module/telegram"
	"github.com/zeromicro/go-zero/core/logx"
)

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

// helpText 返回帮助文案。
func helpText() string {
	return "<b>DockerCopilot 机器人</b>\n" +
		"/ps 或 /containers - 查看容器列表\n" +
		"/images - 查看镜像数量\n" +
		"/start_c <容器名> - 启动容器\n" +
		"/stop_c <容器名> - 停止容器\n" +
		"/restart_c <容器名> - 重启容器\n" +
		"/help - 查看帮助"
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
