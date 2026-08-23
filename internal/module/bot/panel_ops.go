package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

// doUpdate 提交容器更新任务（沿用当前镜像），并回报任务已提交。
func (b *Bot) doUpdate(chatID int64, id, name string) {
	c, ok := b.findContainer(id)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	// 沿用容器当前镜像进行更新（拉取同名 tag 的最新镜像并重建）
	taskID, err := b.ops.Update(c.ID, name, c.Image)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 提交更新失败：%s", err.Error()))
		return
	}
	b.reply(chatID, fmt.Sprintf("🚀 容器 <b>%s</b> 更新任务已提交\n镜像：<code>%s</code>\n任务ID：<code>%s</code>\n更新在后台执行，请稍后用 /ps 查看状态。",
		escapeHTML(name), escapeHTML(shortImage(c.Image)), taskID[:8]))
}

// sendContainerLogs 推送容器最近日志（最后 50 行，限制长度避免超 TG 消息上限）。
func (b *Bot) sendContainerLogs(chatID int64, id, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// TG 单条消息上限约 4096 字符，日志控制在 3500 字节内
	out, err := b.ops.Logs(ctx, id, 50, "", false, 3500)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 获取日志失败：%s", err.Error()))
		return
	}
	out = strings.TrimSpace(out)
	if out == "" {
		out = "(无日志输出)"
	}
	// 截断保护
	if len(out) > 3500 {
		out = out[len(out)-3500:]
	}
	text := fmt.Sprintf("<b>📄 %s 最近日志</b>\n<pre>%s</pre>", escapeHTML(name), escapeHTML(out))
	kb := b.backToPanelKb(id, name)
	b.replyKeyboard(chatID, text, kb)
}

// sendContainerInspect 推送容器详情（镜像/状态/端口/网络/时间）。
func (b *Bot) sendContainerInspect(chatID int64, id, name string) {
	c, ok := b.findContainer(id)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>🔍 容器详情：%s</b>\n", escapeHTML(name)))
	t.WriteString(fmt.Sprintf("ID：<code>%s</code>\n", escapeHTML(shortID(c.ID))))
	t.WriteString(fmt.Sprintf("状态：%s\n", stateLabel(c.State)))
	t.WriteString(fmt.Sprintf("镜像：<code>%s</code>\n", escapeHTML(c.Image)))
	if c.Update {
		t.WriteString("更新：🔺 有可用更新\n")
	} else {
		t.WriteString("更新：✅ 已是最新\n")
	}
	// 端口映射
	if len(c.Ports) > 0 {
		var ports []string
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports = append(ports, fmt.Sprintf("%d→%d/%s", p.PublicPort, p.PrivatePort, p.Type))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			}
		}
		t.WriteString(fmt.Sprintf("端口：<code>%s</code>\n", escapeHTML(strings.Join(ports, ", "))))
	}
	// 网络模式
	if c.HostConfig.NetworkMode != "" {
		t.WriteString(fmt.Sprintf("网络：<code>%s</code>\n", escapeHTML(c.HostConfig.NetworkMode)))
	}
	// 创建时间
	if c.Created > 0 {
		t.WriteString(fmt.Sprintf("创建：%s\n", time.Unix(c.Created, 0).Format("2006-01-02 15:04:05")))
	}
	if c.Status != "" {
		t.WriteString(fmt.Sprintf("运行：%s\n", escapeHTML(c.Status)))
	}
	kb := b.backToPanelKb(id, name)
	b.replyKeyboard(chatID, t.String(), kb)
}

// sendContainerStats 推送容器实时资源占用（CPU/内存），采样一次。
func (b *Bot) sendContainerStats(chatID int64, id, name string) {
	stat, err := b.ops.Stats(id)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 获取资源占用失败：%s", err.Error()))
		return
	}
	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>📊 %s 资源占用</b>\n", escapeHTML(name)))
	t.WriteString(fmt.Sprintf("CPU：%.2f%%\n", stat.CPUPercent))
	t.WriteString(fmt.Sprintf("内存：%.1f MB / %.1f MB（%.1f%%）\n",
		float64(stat.MemUsage)/1024/1024, float64(stat.MemLimit)/1024/1024, stat.MemPercent))
	kb := b.backToPanelKb(id, name)
	b.replyKeyboard(chatID, t.String(), kb)
}

// backToPanelKb 生成"返回容器面板"的键盘。
func (b *Bot) backToPanelKb(id, name string) *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s", id, name)},
	}}}
}

// shortID 截取容器短 ID。
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
