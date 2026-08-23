package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
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

// promptExec 进入容器的交互式 Shell 会话。
// 进入后用户连续发送的每条文本都作为命令在该容器内执行，直到发送 /exit 退出。
// 会话采用"单屏终端"形态：发送一条常驻"终端消息"，之后每条命令的结果都编辑更新到这条消息上，
// 并删除用户发送的命令消息，保持对话干净。
func (b *Bot) promptExec(chatID int64, id, name string) {
	welcome := fmt.Sprintf(
		"🖥 <b>%s</b> Shell 会话已就绪\n\n"+
			"• 直接发送命令即可连续执行（支持管道/重定向）\n"+
			"• <code>cd</code> 会在会话内保持工作目录\n"+
			"• 结果会更新在本条消息，命令消息将被自动清理\n\n"+
			"等待输入命令…", escapeHTML(name))
	// 发送终端消息并记录其 ID，后续所有结果都编辑到这一条上
	msgID, err := b.client.SendMessageReturnID(chatID, welcome, b.shellKb(id, name))
	if err != nil {
		logx.Errorf("Telegram 发送 Shell 终端消息失败 chat=%d: %v", chatID, err)
		b.reply(chatID, "❌ 无法进入 Shell 会话，请稍后重试。")
		return
	}
	b.setShell(chatID, &shellSession{id: id, name: name, workDir: "", resultMsgID: msgID})
}

// shellKb 返回 Shell 会话终端消息下方的内联键盘：查看历史命令 / 退出交互。
func (b *Bot) shellKb(id, name string) *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "📜 历史命令", CallbackData: fmt.Sprintf("shhist|%s|%s", id, name)},
		{Text: "⏹ 退出交互", CallbackData: fmt.Sprintf("shexit|%s|%s", id, name)},
	}}}
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
	case "tag":
		b.doSwitchTag(chatID, p.id, p.name, input)
	default:
		b.reply(chatID, "未知的待处理动作")
	}
}

// runShellCommand 在 Shell 会话中执行一条命令，并保持工作目录连续。
// 单屏终端形态：先删除用户的命令消息，执行后把结果编辑更新到常驻的"终端消息"上。
// cd 连续性：sh -c 里先 cd 到会话记录的目录再执行命令，末尾打印哨兵+pwd，
// 从输出里解析出执行后的工作目录并回写。
func (b *Bot) runShellCommand(chatID int64, s *shellSession, cmd string, cmdMsgID int64) {
	// 立即删除用户发送的命令消息，保持"单屏"干净
	b.deleteMsg(chatID, cmdMsgID)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	// 优化 ls 命令输出：自动添加 -C 参数实现多列显示（类似真实终端）
	execCmd := cmd
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "ls" || (strings.HasPrefix(trimmed, "ls ") && !strings.Contains(trimmed, " -")) {
		// 仅对纯 ls 或 ls <路径>（无选项参数）添加 -C
		execCmd = strings.Replace(cmd, "ls", "ls -C", 1)
	}

	// 哨兵：用于从输出末尾提取执行后的工作目录，避免与用户输出混淆
	const marker = "__DC_PWD__:"
	workDir := s.workDir
	if workDir == "" {
		workDir = "." // 容器默认工作目录
	}
	// 组合脚本：cd 到当前目录 -> 执行用户命令 -> 无论成败都打印哨兵+pwd
	script := fmt.Sprintf("cd %s 2>/dev/null; %s; __ec=$?; printf '\\n%s%%s\\n' \"$(pwd)\"; exit $__ec",
		shellQuote(workDir), execCmd, marker)

	res, err := b.ops.Exec(ctx, s.id, []string{"sh", "-c", script}, "", "", 30, 8000)
	if err != nil {
		b.updateShellMsg(chatID, s, fmt.Sprintf("❌ 执行失败：%s", escapeHTML(err.Error())))
		return
	}

	// 从输出中剥离哨兵行，解析新的工作目录
	out := res.Output
	newDir := s.workDir
	if idx := strings.LastIndex(out, marker); idx >= 0 {
		tail := out[idx+len(marker):]
		if nl := strings.IndexByte(tail, '\n'); nl >= 0 {
			tail = tail[:nl]
		}
		if d := strings.TrimSpace(tail); d != "" {
			newDir = d
		}
		// 去掉哨兵行及其前导换行，保留纯净的命令输出
		out = strings.TrimRight(out[:idx], "\r\n")
	}
	if newDir != "" {
		s.workDir = newDir
	}
	// 记录命令历史（最多保留 20 条）
	s.history = append(s.history, cmd)
	if len(s.history) > 20 {
		s.history = s.history[len(s.history)-20:]
	}
	b.setShell(chatID, s)

	out = strings.TrimSpace(out)
	if out == "" {
		out = "(无输出)"
	}
	if len(out) > 3500 {
		out = out[:3500] + "\n...(输出过长已截断)"
	}
	dirLabel := s.workDir
	if dirLabel == "" {
		dirLabel = "~"
	}
	body := fmt.Sprintf("🖥 <b>%s</b>  <code>%s</code>\n<b>$</b> <code>%s</code>（退出码 %d）\n<pre>%s</pre>",
		escapeHTML(s.name), escapeHTML(dirLabel), escapeHTML(cmd), res.ExitCode, escapeHTML(out))
	b.updateShellMsg(chatID, s, body)
}

// updateShellMsg 把内容编辑更新到会话的终端消息上（始终保持单屏），并带上会话键盘。
func (b *Bot) updateShellMsg(chatID int64, s *shellSession, text string) {
	b.editMessageKeyboard(chatID, s.resultMsgID, text, b.shellKb(s.id, s.name))
}

// finishShell 结束 Shell 会话：清除会话状态，并把终端消息更新为已退出提示。
func (b *Bot) finishShell(chatID int64, s *shellSession) {
	b.clearShell(chatID)
	text := fmt.Sprintf("✅ 已退出容器 <b>%s</b> 的 Shell 会话。", escapeHTML(s.name))
	// 退出后移除键盘
	b.editMessageKeyboard(chatID, s.resultMsgID, text, nil)
}

// showShellHistory 把历史命令列表编辑更新到终端消息上。
func (b *Bot) showShellHistory(chatID int64, s *shellSession) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📜 <b>%s</b> 历史命令（最近 %d 条）\n\n", escapeHTML(s.name), len(s.history)))
	if len(s.history) == 0 {
		sb.WriteString("（暂无历史命令）")
	} else {
		for i, h := range s.history {
			sb.WriteString(fmt.Sprintf("%d. <code>%s</code>\n", i+1, escapeHTML(h)))
		}
	}
	sb.WriteString("\n继续发送命令即可执行。")
	b.updateShellMsg(chatID, s, sb.String())
}

// shellQuote 用单引号安全包裹路径，转义其中的单引号，防止命令注入/路径含空格出错。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
