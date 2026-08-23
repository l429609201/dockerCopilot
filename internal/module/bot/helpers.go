package bot

import (
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
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
	return "<b>DockerCopilot 机器人</b>\n\n" +
		"<b>容器管理</b>\n" +
		"/ps 或 /containers - 查看容器列表\n" +
		"/start_c &lt;容器名&gt; - 启动容器\n" +
		"/stop_c &lt;容器名&gt; - 停止容器\n" +
		"/restart_c &lt;容器名&gt; - 重启容器\n\n" +
		"<b>镜像与系统</b>\n" +
		"/images - 查看镜像列表（名称、大小、是否在用）\n" +
		"/sys - 查看系统概览（容器/镜像统计、磁盘占用）\n\n" +
		"<b>更新管理</b>\n" +
		"/check_updates - 检查所有容器的镜像更新\n" +
		"/update_all - 批量更新所有有更新的容器\n\n" +
		"<b>Compose 项目</b>\n" +
		"/compose - 管理 Docker Compose 项目\n\n" +
		"<b>提示</b>：在 /ps 列表点「⚙管理」进入单容器面板，\n" +
		"可用：启动/停止/重启/暂停/恢复、更新、切换标签、\n" +
		"查看日志、详情、资源、命令行、重命名、删除等操作。"
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
