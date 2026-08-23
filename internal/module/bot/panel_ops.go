package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

// doUpdate 提交容器更新任务（沿用当前镜像），并在原管理面板上持续显示进度。
// messageID > 0 时编辑原消息显示进度，否则发送新消息。
func (b *Bot) doUpdate(chatID int64, id, name string, messageID int64) {
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

	// 启动进度监听 goroutine，持续编辑消息显示进度
	go b.watchUpdateProgress(chatID, messageID, taskID, name, c.Image)
}

// sendContainerLogs 推送容器最近日志（最后 50 行，限制长度避免超 TG 消息上限）。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerLogs(chatID int64, id, name string, messageID int64) {
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
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// sendContainerInspect 推送容器详情（镜像/状态/端口/网络/时间）。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerInspect(chatID int64, id, name string, messageID int64) {
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
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
}

// sendContainerStats 推送容器实时资源占用（CPU/内存），采样一次。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerStats(chatID int64, id, name string, messageID int64) {
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
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
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

// watchUpdateProgress 监听容器更新任务进度，持续编辑消息显示进度条和状态。
// 适配 Telegram 每条消息最多每秒编辑一次的频率限制。
func (b *Bot) watchUpdateProgress(chatID int64, messageID int64, taskID, name, image string) {
	ticker := time.NewTicker(1 * time.Second) // Telegram 编辑限制：每秒最多一次
	defer ticker.Stop()

	lastProgress := -1 // 记录上次进度，避免重复编辑
	maxWait := 300     // 最长等待5分钟，防止任务卡死导致 goroutine 泄漏
	elapsed := 0

	for {
		select {
		case <-ticker.C:
			elapsed++
			if elapsed > maxWait {
				// 超时，停止监听
				return
			}

			// 从 ProgressStore 获取任务进度
			progress, exists := b.svcCtx.GetProgress(taskID)
			if !exists {
				// 任务不存在，可能已完成或失败
				return
			}

			// 进度未变化且未完成，跳过编辑
			if progress.Percentage == lastProgress && !progress.IsDone {
				continue
			}
			lastProgress = progress.Percentage

			// 构建进度消息
			var text strings.Builder
			text.WriteString(fmt.Sprintf("<b>🚀 正在更新：%s</b>\n\n", escapeHTML(name)))
			text.WriteString(fmt.Sprintf("镜像：<code>%s</code>\n", escapeHTML(shortImage(image))))
			text.WriteString(fmt.Sprintf("任务ID：<code>%s</code>\n\n", taskID[:8]))

			// 绘制进度条
			text.WriteString(b.renderProgressBar(progress.Percentage))
			text.WriteString("\n\n")

			// 显示当前状态
			if progress.Message != "" {
				text.WriteString(fmt.Sprintf("状态：%s\n", escapeHTML(progress.Message)))
			}
			if progress.DetailMsg != "" {
				text.WriteString(fmt.Sprintf("详情：%s\n", escapeHTML(progress.DetailMsg)))
			}

			// 任务完成
			if progress.IsDone {
				if progress.Failed {
					text.WriteString("\n❌ <b>更新失败</b>")
				} else if progress.Canceled {
					text.WriteString("\n⚠️ <b>更新已取消</b>")
				} else {
					text.WriteString("\n✅ <b>更新完成</b>")
				}

				// 添加返回按钮
				kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
					{Text: "📦 查看容器列表", CallbackData: "menu|ps|0"},
				}}}

				// 最后一次编辑
				if messageID > 0 {
					b.editMessageKeyboard(chatID, messageID, text.String(), kb)
				} else {
					b.replyKeyboard(chatID, text.String(), kb)
				}
				return
			}

			// 编辑消息显示进度
			if messageID > 0 {
				b.editMessage(chatID, messageID, text.String())
			} else {
				// 如果没有 messageID，发送新消息（不应该走到这里）
				b.reply(chatID, text.String())
				return
			}
		}
	}
}

// renderProgressBar 渲染文本进度条，百分比 0-100。
// 示例：[████████░░] 80%
func (b *Bot) renderProgressBar(percentage int) string {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}

	const totalBlocks = 10
	filledBlocks := percentage * totalBlocks / 100
	emptyBlocks := totalBlocks - filledBlocks

	bar := strings.Repeat("█", filledBlocks) + strings.Repeat("░", emptyBlocks)
	return fmt.Sprintf("[%s] %d%%", bar, percentage)
}
