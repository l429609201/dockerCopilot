package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

// doUpdate 提交容器更新任务（沿用当前镜像），并在原管理面板上持续显示进度。
// messageID > 0 时编辑原消息显示进度，否则发送新消息。hostID 定位容器所属主机。
func (b *Bot) doUpdate(chatID int64, id, name, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	// 优先使用 CreateImage（创建时的镜像名），避免 Image 字段在镜像更新后变空或变成 SHA256
	imageToUse := c.CreateImage
	if imageToUse == "" {
		imageToUse = c.Image // 降级使用 Image 字段
	}
	// 沿用容器当前镜像进行更新（拉取同名 tag 的最新镜像并重建），按主机路由
	taskID, err := containerops.NewForHost(b.svcCtx, hostID).UpdateWithOptions(c.ID, imageToUse, containerops.UpdateOptions{})
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 提交更新失败：%s", err.Error()))
		return
	}

	// 启动进度监听 goroutine，持续编辑消息显示进度
	go b.watchUpdateProgress(chatID, messageID, taskID, name, imageToUse)
}

// sendContainerLogs 推送容器最近日志（最后 50 行，限制长度避免超 TG 消息上限）。
// messageID > 0 时编辑原消息，否则发送新消息。hostID 定位容器所属主机。
func (b *Bot) sendContainerLogs(chatID int64, id, name, hostID string, messageID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ops := containerops.NewForHost(b.svcCtx, hostID)
	hs := "|" + encodeHostRef(hostID)
	// TG 单条消息上限约 4096 字符，日志控制在 3500 字节内
	out, err := ops.Logs(ctx, id, 50, "", false, 3500)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 获取日志失败：%s", err.Error()))
		return
	}
	out = strings.TrimSpace(out)
	if out == "" {
		out = "(无日志输出)"
	}
	// 截断保护
	runes := []rune(strings.ToValidUTF8(out, ""))
	if len(runes) > 1500 {
		runes = runes[len(runes)-1500:]
	}
	out = string(runes)
	text := fmt.Sprintf("<b>📄 %s 最近日志</b>\n<pre>%s</pre>", escapeHTML(truncName(name, 100)), escapeHTML(out))
	// 键盘：下载完整日志 + 返回管理
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "⬇ 下载完整日志", CallbackData: fmt.Sprintf("logdl|%s|%s%s", id, name, hs)}},
		{{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s%s", id, name, hs)}},
	}}
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// sendContainerLogFile 拉取更多行日志并作为 .log 文档发送（突破 TG 文本消息 4096 上限）。
func (b *Bot) sendContainerLogFile(chatID int64, id, name, hostID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// 文件形式可容纳更多内容：取最近 2000 行，上限 2MB
	out, err := containerops.NewForHost(b.svcCtx, hostID).Logs(ctx, id, 2000, "", true, 2*1024*1024)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 获取日志失败：%s", err.Error()))
		return
	}
	if strings.TrimSpace(out) == "" {
		b.reply(chatID, "⚠️ 该容器暂无日志输出")
		return
	}
	// 生成安全文件名：容器名_时间戳.log
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ' ' || r == ':' {
			return '_'
		}
		return r
	}, name)
	if safeName == "" {
		safeName = shortID(id)
	}
	filename := fmt.Sprintf("%s_%s.log", safeName, time.Now().Format("20060102-150405"))
	caption := fmt.Sprintf("📄 <b>%s</b> 日志（最近 2000 行）", escapeHTML(name))
	if err := b.client.SendDocument(chatID, filename, []byte(out), caption); err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 发送日志文件失败：%s", err.Error()))
	}
}

// sendContainerInspect 推送容器详情（镜像/状态/端口/网络/时间）。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerInspect(chatID int64, id, name, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>🔍 容器详情：%s</b>\n", escapeHTML(truncName(name, 100))))
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
	kb := b.backToPanelKb(id, name, hostID)
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
}

// sendContainerStats 推送容器实时资源占用（CPU/内存），采样一次。
// messageID > 0 时编辑原消息，否则发送新消息。hostID 定位容器所属主机。
func (b *Bot) sendContainerStats(chatID int64, id, name, hostID string, messageID int64) {
	stat, err := containerops.NewForHost(b.svcCtx, hostID).Stats(id)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 获取资源占用失败：%s", err.Error()))
		return
	}
	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>📊 %s 资源占用</b>\n", escapeHTML(truncName(name, 100))))
	t.WriteString(fmt.Sprintf("CPU：%.2f%%\n", stat.CPUPercent))
	t.WriteString(fmt.Sprintf("内存：%.1f MB / %.1f MB（%.1f%%）\n",
		float64(stat.MemUsage)/1024/1024, float64(stat.MemLimit)/1024/1024, stat.MemPercent))
	kb := b.backToPanelKb(id, name, hostID)
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
}

// backToPanelKb 生成"返回容器面板"的键盘。hostID 决定回到哪个主机的面板。
func (b *Bot) backToPanelKb(id, name, hostID string) *telegram.InlineKeyboardMarkup {
	hs := "|" + encodeHostRef(hostID)
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s%s", id, name, hs)},
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
// 限频并去重；只有明确终态才显示完成，缺失进度绝不猜测成功。
func (b *Bot) watchUpdateProgress(chatID int64, messageID int64, taskID, name, image string) {
	// 无面板时只创建一次消息，后续始终编辑，避免刷屏。
	if messageID <= 0 {
		var err error
		messageID, err = b.client.SendMessageReturnID(chatID, "⏳ 更新任务已提交，等待进度…", nil)
		if err != nil {
			return
		}
	}
	b.mu.Lock()
	stopCh := b.stopCh
	b.mu.Unlock()
	// 不按五分钟截断慢速拉取；Bot 停止时释放监听，无进度不推断成功。
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	lastText := ""
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			progress, exists := b.svcCtx.GetProgress(taskID)
			if !exists {
				text := "⚠️ 暂无任务进度，尚未确认更新结果；正在继续等待。"
				if text != lastText {
					b.editMessage(chatID, messageID, text)
					lastText = text
				}
				continue
			}

			// 构建进度消息
			var text strings.Builder
			text.WriteString(fmt.Sprintf("<b>🚀 正在更新：%s</b>\n\n", escapeHTML(truncName(name, 100))))
			text.WriteString(fmt.Sprintf("镜像：<code>%s</code>\n", escapeHTML(truncName(image, 200))))
			text.WriteString(fmt.Sprintf("任务ID：<code>%s</code>\n\n", escapeHTML(truncName(taskID, 36))))

			// 绘制进度条
			text.WriteString(b.renderProgressBar(progress.Percentage))
			text.WriteString("\n\n")

			// 显示当前状态
			if progress.Message != "" {
				text.WriteString(fmt.Sprintf("状态：%s\n", escapeHTML(truncName(progress.Message, 200))))
			}
			if progress.DetailMsg != "" {
				text.WriteString(fmt.Sprintf("详情：%s\n", escapeHTML(truncName(progress.DetailMsg, 500))))
			}

			// 任务完成
			if progress.IsDone {
				if progress.Failed {
					text.WriteString("\n❌ <b>更新失败</b>")
				} else if progress.Canceled {
					text.WriteString("\n⚠️ <b>更新已取消</b>")
				} else if updateHandedOff(progress.Message, progress.DetailMsg) {
					// 自更新的 IsDone 仅表示已交接，重启后的健康状态尚未验证。
					text.WriteString("\n⏳ <b>辅助容器已接管，等待重启</b>\n尚未确认更新成功，请稍后查看容器状态。")
				} else {
					text.WriteString("\n✅ <b>更新完成</b>\n\n")

					// 【增强】显示镜像详细信息
					text.WriteString("<b>📦 镜像详情</b>\n")
					if progress.ImageName != "" {
						text.WriteString(fmt.Sprintf("名称：<code>%s</code>\n", escapeHTML(truncName(progress.ImageName, 200))))
					}
					if progress.ImageTag != "" {
						text.WriteString(fmt.Sprintf("标签：<code>%s</code>\n", escapeHTML(truncName(progress.ImageTag, 128))))
					}

					// 显示旧镜像信息
					if progress.OldImageDigest != "" {
						text.WriteString(fmt.Sprintf("\n<b>📋 旧镜像</b>\n"))
						text.WriteString(fmt.Sprintf("SHA256：<code>%s</code>\n", escapeHTML(shortenDigest(progress.OldImageDigest))))
						if progress.OldImageSize > 0 {
							text.WriteString(fmt.Sprintf("大小：%s\n", formatBytes(progress.OldImageSize)))
						}
					}

					// 显示新镜像信息
					if progress.NewImageDigest != "" {
						text.WriteString(fmt.Sprintf("\n<b>🆕 新镜像</b>\n"))
						text.WriteString(fmt.Sprintf("SHA256：<code>%s</code>\n", escapeHTML(shortenDigest(progress.NewImageDigest))))
						if progress.NewImageSize > 0 {
							text.WriteString(fmt.Sprintf("大小：%s\n", formatBytes(progress.NewImageSize)))
						}

						// 显示大小变化
						if progress.OldImageSize > 0 && progress.NewImageSize > 0 {
							sizeDiff := progress.NewImageSize - progress.OldImageSize
							if sizeDiff > 0 {
								text.WriteString(fmt.Sprintf("变化：<b>+%s</b> ⬆️\n", formatBytes(sizeDiff)))
							} else if sizeDiff < 0 {
								text.WriteString(fmt.Sprintf("变化：<b>-%s</b> ⬇️\n", formatBytes(-sizeDiff)))
							} else {
								text.WriteString("变化：<b>无变化</b>\n")
							}
						}
					}
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

			// 按最终显示内容去重，状态文字变化也能展示，最多三秒一次。
			shown := text.String()
			if shown != lastText {
				b.editMessage(chatID, messageID, shown)
				lastText = shown
			}
		}
	}
}

// updateHandedOff 识别现有自更新协议中的交信号，不将其当作重启成功。
func updateHandedOff(message, detail string) bool {
	return strings.Contains(message, "辅助容器已接管") || strings.Contains(detail, "辅助容器已接管")
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

// shortenDigest 缩短 SHA256 摘要，只显示前 12 位，便于阅读
func shortenDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// formatBytes 将字节数格式化为人类可读的大小（KB, MB, GB）
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
