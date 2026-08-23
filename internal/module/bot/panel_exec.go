package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// sendTagSwitch 展示可切换的镜像标签：列出本地与当前镜像同名的其它 tag 作为按钮，
// 并提供"手动输入标签"入口。切换标签本质是用目标 tag 镜像重建容器。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendTagSwitch(chatID int64, id, name string, messageID int64) {
	c, ok := b.findContainer(id)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	// 解析当前镜像名（去掉 tag）
	curImage := c.Image
	repo := curImage
	if idx := strings.LastIndex(curImage, ":"); idx >= 0 && !strings.Contains(curImage[idx:], "/") {
		repo = curImage[:idx]
	}

	// 查找本地同名镜像的所有 tag
	images, err := utiles.GetImagesList(b.svcCtx)
	var rows [][]telegram.InlineKeyboardButton
	if err == nil {
		seen := map[string]bool{}
		for _, img := range images {
			if img.ImageName != repo || img.ImageTag == "" || img.ImageTag == "None" {
				continue
			}
			full := repo + ":" + img.ImageTag
			if seen[full] || full == curImage {
				continue
			}
			seen[full] = true
			rows = append(rows, []telegram.InlineKeyboardButton{
				{Text: "🏷 " + img.ImageTag, CallbackData: fmt.Sprintf("dotag|%s|%s|%s", id, name, img.ImageTag)},
			})
		}
	}

	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>🏷 切换标签：%s</b>\n", escapeHTML(name)))
	t.WriteString(fmt.Sprintf("当前镜像：<code>%s</code>\n", escapeHTML(curImage)))
	if len(rows) > 0 {
		t.WriteString("\n选择本地已有的标签，或手动输入：")
	} else {
		t.WriteString("\n未找到本地其它标签，可手动输入目标标签：")
	}

	// 手动输入 + 返回
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "✏ 手动输入标签", CallbackData: fmt.Sprintf("tagin|%s|%s", id, name)},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s", id, name)},
	})
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
}

// doSwitchTag 用指定 tag 重建容器：拼出 repo:tag 后走更新流程。
func (b *Bot) doSwitchTag(chatID int64, id, name, tag string) {
	c, ok := b.findContainer(id)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	repo := c.Image
	if idx := strings.LastIndex(c.Image, ":"); idx >= 0 && !strings.Contains(c.Image[idx:], "/") {
		repo = c.Image[:idx]
	}
	target := repo + ":" + strings.TrimSpace(tag)
	taskID, err := b.ops.Update(c.ID, name, target)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 提交切换失败：%s", err.Error()))
		return
	}
	b.reply(chatID, fmt.Sprintf("🚀 容器 <b>%s</b> 切换标签任务已提交\n目标镜像：<code>%s</code>\n任务ID：<code>%s</code>\n后台执行中，请稍后用 /ps 查看。",
		escapeHTML(name), escapeHTML(target), taskID[:8]))
}

// promptRename 进入重命名等待：提示用户发送新名称。
func (b *Bot) promptRename(chatID int64, id, name string) {
	b.setPending(chatID, &pendingAction{kind: "rename", id: id, name: name})
	b.reply(chatID, fmt.Sprintf("✏ 请发送容器 <b>%s</b> 的新名称（发送 /cancel 取消）：", escapeHTML(name)))
}

// promptExec 进入命令行等待：提示用户发送要在容器内执行的命令。
func (b *Bot) promptExec(chatID int64, id, name string) {
	b.setPending(chatID, &pendingAction{kind: "exec", id: id, name: name})
	b.reply(chatID, fmt.Sprintf("💻 请发送要在容器 <b>%s</b> 内执行的命令（发送 /cancel 取消）：\n例如：<code>ls -al /</code>", escapeHTML(name)))
}

// promptTagInput 进入标签输入等待：提示用户发送目标标签。
func (b *Bot) promptTagInput(chatID int64, id, name string) {
	b.setPending(chatID, &pendingAction{kind: "tag", id: id, name: name})
	b.reply(chatID, fmt.Sprintf("🏷 请发送容器 <b>%s</b> 要切换到的镜像标签（发送 /cancel 取消）：\n例如：<code>latest</code> 或 <code>v1.2.3</code>", escapeHTML(name)))
}

// completePending 根据待输入动作类型完成操作。
func (b *Bot) completePending(chatID int64, p *pendingAction, input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		b.reply(chatID, "输入为空，已取消。")
		return
	}
	switch p.kind {
	case "rename":
		if err := b.ops.Rename(p.id, input); err != nil {
			b.reply(chatID, fmt.Sprintf("❌ 重命名失败：%s", err.Error()))
			return
		}
		b.reply(chatID, fmt.Sprintf("✅ 容器已重命名为 <b>%s</b>", escapeHTML(input)))
	case "exec":
		b.doExec(chatID, p.id, p.name, input)
	case "tag":
		b.doSwitchTag(chatID, p.id, p.name, input)
	default:
		b.reply(chatID, "未知的待处理动作")
	}
}

// doExec 在容器内执行命令并返回输出。
// 通过 sh -c 包裹用户命令，以支持管道/重定向等常见用法。
func (b *Bot) doExec(chatID int64, id, name, cmd string) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	res, err := b.ops.Exec(ctx, id, []string{"sh", "-c", cmd}, "", "", 30, 3500)
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 执行失败：%s", err.Error()))
		return
	}
	out := strings.TrimSpace(res.Output)
	if out == "" {
		out = "(无输出)"
	}
	if len(out) > 3500 {
		out = out[:3500]
	}
	text := fmt.Sprintf("<b>💻 %s 命令输出</b>（退出码 %d）\n<code>%s</code>\n<pre>%s</pre>",
		escapeHTML(name), res.ExitCode, escapeHTML(cmd), escapeHTML(out))
	kb := b.backToPanelKb(id, name)
	b.replyKeyboard(chatID, text, kb)
}
