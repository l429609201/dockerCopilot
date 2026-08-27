package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// sendTagSwitch 展示可切换的镜像标签：列出本地与当前镜像同名的其它 tag 作为按钮，
// 并提供"手动输入标签"入口。切换标签本质是用目标 tag 镜像重建容器。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendTagSwitch(chatID int64, id, name, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	hs := "|" + b.svcCtx.DockerManager.HostCode(hostID)
	// 解析当前镜像名（去掉 tag），优先使用 CreateImage
	curImage := c.CreateImage
	if curImage == "" {
		curImage = c.Image // 降级使用 Image 字段
	}
	repo := curImage
	if idx := strings.LastIndex(curImage, ":"); idx >= 0 && !strings.Contains(curImage[idx:], "/") {
		repo = curImage[:idx]
	}

	// 查找该容器所属主机上同名镜像的所有 tag（按主机路由，远程容器查远程镜像）
	images, err := utiles.GetImagesListFromHost(b.svcCtx, hostID)
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
				{Text: "🏷 " + img.ImageTag, CallbackData: fmt.Sprintf("dotag|%s|%s|%s%s", id, name, img.ImageTag, hs)},
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
		{Text: "✏ 手动输入标签", CallbackData: fmt.Sprintf("tagin|%s|%s%s", id, name, hs)},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s%s", id, name, hs)},
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
// messageID > 0 时把进度编辑到原消息（面板点击），否则发新消息（文本输入切标签）。
func (b *Bot) doSwitchTag(chatID int64, id, name, tag, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 容器不存在或已被删除", b.backToPanelKb(id, name, hostID))
		return
	}
	repo := c.Image
	if idx := strings.LastIndex(c.Image, ":"); idx >= 0 && !strings.Contains(c.Image[idx:], "/") {
		repo = c.Image[:idx]
	}
	target := repo + ":" + strings.TrimSpace(tag)
	taskID, err := containerops.NewForHost(b.svcCtx, hostID).Update(c.ID, name, target)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID,
			fmt.Sprintf("❌ 提交切换失败：%s", escapeHTML(err.Error())), b.backToPanelKb(id, name, hostID))
		return
	}
	// 复用更新进度监听：把切标签进度持续编辑到原消息上
	go b.watchUpdateProgress(chatID, messageID, taskID, name, target)
}

// promptRename 进入重命名等待：提示用户发送新名称。hostID 记入待处理动作。
func (b *Bot) promptRename(chatID int64, id, name, hostID string) {
	b.setPending(chatID, &pendingAction{kind: "rename", id: id, name: name, hostID: hostID})
	b.reply(chatID, fmt.Sprintf("✏ 请发送容器 <b>%s</b> 的新名称（发送 /cancel 取消）：", escapeHTML(name)))
}

// promptExec 进入容器的交互式 Shell 会话。
// 进入后用户连续发送的每条文本都作为命令在该容器内执行，直到发送 /exit 退出。
// 会话采用"单屏终端"形态：直接在面板消息(panelMsgID)上编辑成终端界面，
// 之后每条命令的结果都编辑更新到这条消息上，退出后再把这条消息恢复为容器面板菜单。
// panelMsgID > 0 时复用该消息（不新发消息）；为 0 时回退发送新消息。
func (b *Bot) promptExec(chatID int64, id, name, hostID string, panelMsgID int64) {
	welcome := fmt.Sprintf(
		"🖥 <b>%s</b> Shell 会话已就绪\n\n"+
			"• 直接发送命令即可连续执行（支持管道/重定向）\n"+
			"• <code>cd</code> 会在会话内保持工作目录\n"+
			"• 结果会更新在本条消息，命令消息将被自动清理\n\n"+
			"等待输入命令…", escapeHTML(name))
	var msgID int64
	if panelMsgID > 0 {
		// 复用面板消息：编辑为终端界面，会话结果都更新在这一条上
		b.editMessageKeyboard(chatID, panelMsgID, welcome, b.shellKb(id, name, hostID))
		msgID = panelMsgID
	} else {
		// 无面板消息（如命令触发）：新发一条终端消息
		var err error
		msgID, err = b.client.SendMessageReturnID(chatID, welcome, b.shellKb(id, name, hostID))
		if err != nil {
			logx.Errorf("Telegram 发送 Shell 终端消息失败 chat=%d: %v", chatID, err)
			b.reply(chatID, "❌ 无法进入 Shell 会话，请稍后重试。")
			return
		}
	}
	b.setShell(chatID, &shellSession{id: id, name: name, hostID: hostID, workDir: "", resultMsgID: msgID})
}

// shellKb 返回 Shell 会话终端消息下方的内联键盘：查看历史命令 / 退出交互。
func (b *Bot) shellKb(id, name, hostID string) *telegram.InlineKeyboardMarkup {
	hs := "|" + b.svcCtx.DockerManager.HostCode(hostID)
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "📜 历史命令", CallbackData: fmt.Sprintf("shhist|%s|%s%s", id, name, hs)},
		{Text: "⏹ 退出交互", CallbackData: fmt.Sprintf("shexit|%s|%s%s", id, name, hs)},
	}}}
}

// promptTagInput 进入标签输入等待：提示用户发送目标标签。hostID 记入待处理动作。
func (b *Bot) promptTagInput(chatID int64, id, name, hostID string) {
	b.setPending(chatID, &pendingAction{kind: "tag", id: id, name: name, hostID: hostID})
	b.reply(chatID, fmt.Sprintf("🏷 请发送容器 <b>%s</b> 要切换到的镜像标签（发送 /cancel 取消）：\n例如：<code>latest</code> 或 <code>v1.2.3</code>", escapeHTML(name)))
}

// completePending 根据待输入动作类型完成操作。
// 由文本消息触发，无原始面板 messageID，故结果发新消息并附「返回管理」键盘（用新名称定位面板）。
func (b *Bot) completePending(chatID int64, p *pendingAction, input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		b.reply(chatID, "输入为空，已取消。")
		return
	}
	switch p.kind {
	case "rename":
		// 按容器所属主机执行重命名
		if err := containerops.NewForHost(b.svcCtx, p.hostID).Rename(p.id, input); err != nil {
			b.replyKeyboard(chatID, fmt.Sprintf("❌ 重命名失败：%s", escapeHTML(err.Error())), b.backToPanelKb(p.id, p.name, p.hostID))
			return
		}
		// 重命名成功后容器名已变，用新名称构造返回面板按钮
		b.replyKeyboard(chatID, fmt.Sprintf("✅ 容器已重命名为 <b>%s</b>", escapeHTML(input)), b.backToPanelKb(p.id, input, p.hostID))
	case "tag":
		// 文本输入切标签无面板 messageID，传 0 发新消息
		b.doSwitchTag(chatID, p.id, p.name, input, p.hostID, 0)
	default:
		b.reply(chatID, "未知的待处理动作")
	}
}

// runShellCommand 在容器内执行一条命令，并保持工作目录连续。
// 采用「一条命令 = 一次非 TTY exec」模型：非 TTY 不回显命令、无提示符、无 ANSI 控制序列，
// 命令结束时输出流自然 EOF 即判定完成，无需静默超时；退出码由 exec inspect 精确获取。
// cd 连续性：sh -c 里先 cd 到会话记录的目录再执行，末尾追加哨兵行携带新工作目录。
// 单屏终端形态：先删除用户的命令消息，执行后把结果编辑更新到常驻的"终端消息"上。
func (b *Bot) runShellCommand(chatID int64, s *shellSession, cmd string, cmdMsgID int64) {
	// 立即删除用户发送的命令消息，保持"单屏"干净
	b.deleteMsg(chatID, cmdMsgID)

	// 优化 ls 命令输出：自动添加 -C 参数实现多列显示（类似真实终端）
	execCmd := cmd
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "ls" || (strings.HasPrefix(trimmed, "ls ") && !strings.Contains(trimmed, " -")) {
		// 仅对纯 ls 或 ls <路径>（无选项参数）添加 -C
		execCmd = strings.Replace(cmd, "ls", "ls -C", 1)
	}

	// 哨兵标记：命令执行完毕后由 shell 打印，格式为 <标记>|<新工作目录>。
	// 非 TTY 下脚本本身不会被回显，哨兵只出现在真实输出末尾，可安全解析后剥离。
	sentinel := fmt.Sprintf("__DC_END_%d__", time.Now().UnixNano())

	// 组装一次性脚本：先 cd 到会话工作目录（连续性），执行用户命令，
	// 再打印哨兵 + 当前目录。退出码由 ContainerExecInspect 获取，无需脚本携带。
	var script strings.Builder
	if s.workDir != "" && s.workDir != "." {
		script.WriteString(fmt.Sprintf("cd %s 2>/dev/null; ", shellQuote(s.workDir)))
	}
	script.WriteString(execCmd)
	script.WriteString(fmt.Sprintf("; printf '\\n%s|%%s\\n' \"$(pwd)\"", sentinel))

	// 创建非 TTY exec，按优先级尝试可用 shell：bash > sh > ash
	ptyShell := newPtyShell(b.svcCtx, s.id, s.hostID, chatID, s.resultMsgID)
	var startErr error
	for _, sh := range []string{"/bin/bash", "/bin/sh", "/bin/ash"} {
		startErr = ptyShell.Start([]string{sh, "-c", script.String()})
		if startErr == nil {
			break
		}
	}
	if startErr != nil {
		b.updateShellMsg(chatID, s, fmt.Sprintf("❌ 启动终端失败：%s", escapeHTML(startErr.Error())))
		return
	}
	defer ptyShell.Close()

	// 后台解复用输出流，done 关闭即表示命令执行结束（流 EOF）
	done := ptyShell.StartPump()

	// 流式刷新：每秒最多一次编辑消息（Telegram 频率限制），直到 EOF 或超时
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	var lastShown string
	timedOut := false

loop:
	for {
		select {
		case <-done:
			break loop
		case <-timeout:
			timedOut = true
			break loop
		case <-ticker.C:
			body, _ := splitSentinel(ptyShell.GetFullOutput(), sentinel)
			shown := strings.TrimSpace(body)
			if shown != "" && shown != lastShown {
				lastShown = shown
				b.updateShellMsgWithOutput(chatID, s, cmd, shown, -1)
			}
		}
	}

	// 收尾：剥离哨兵行，取出正文与新工作目录
	body, newDir := splitSentinel(ptyShell.GetFullOutput(), sentinel)
	if newDir != "" {
		s.workDir = newDir
	}
	b.recordShellHistory(chatID, s, cmd)

	out := strings.TrimSpace(body)
	if timedOut {
		if out == "" {
			out = "(命令执行超时，无输出)"
		}
		b.updateShellMsgWithOutput(chatID, s, cmd, out+"\n...(执行超时)", -1)
		return
	}
	if out == "" {
		out = "(无输出)"
	}
	b.updateShellMsgWithOutput(chatID, s, cmd, out, ptyShell.ExitCode())
}

// splitSentinel 从输出里剥离末尾的哨兵行 "<sentinel>|<工作目录>"。
// 返回哨兵之前的正文，以及解析出的新工作目录（未出现哨兵时目录为空、正文原样返回）。
func splitSentinel(full, sentinel string) (body, dir string) {
	idx := strings.Index(full, sentinel+"|")
	if idx < 0 {
		return full, ""
	}
	body = full[:idx]
	rest := full[idx+len(sentinel)+1:] // 跳过 "sentinel|"
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	return body, strings.TrimSpace(rest)
}

// recordShellHistory 记录命令历史（最多保留 20 条）并持久化会话。
func (b *Bot) recordShellHistory(chatID int64, s *shellSession, cmd string) {
	s.history = append(s.history, cmd)
	if len(s.history) > 20 {
		s.history = s.history[len(s.history)-20:]
	}
	b.setShell(chatID, s)
}

// updateShellMsg 把内容编辑更新到会话的终端消息上（始终保持单屏），并带上会话键盘。
func (b *Bot) updateShellMsg(chatID int64, s *shellSession, text string) {
	b.editMessageKeyboard(chatID, s.resultMsgID, text, b.shellKb(s.id, s.name, s.hostID))
}

// updateShellMsgWithOutput 实时更新终端消息（用于 PTY 流式输出）
func (b *Bot) updateShellMsgWithOutput(chatID int64, s *shellSession, cmd, output string, exitCode int) {
	out := strings.TrimSpace(output)
	if out == "" {
		out = "(执行中...)"
	}
	if len(out) > 3500 {
		out = out[:3500] + "\n...(输出过长已截断)"
	}

	dirLabel := s.workDir
	if dirLabel == "" {
		dirLabel = "~"
	}

	var body string
	if exitCode >= 0 {
		// 命令已完成
		body = fmt.Sprintf("🖥 <b>%s</b>  <code>%s</code>\n<b>$</b> <code>%s</code>（退出码 %d）\n<pre>%s</pre>",
			escapeHTML(s.name), escapeHTML(dirLabel), escapeHTML(cmd), exitCode, escapeHTML(out))
	} else {
		// 命令执行中
		body = fmt.Sprintf("🖥 <b>%s</b>  <code>%s</code>\n<b>$</b> <code>%s</code>\n<pre>%s</pre>",
			escapeHTML(s.name), escapeHTML(dirLabel), escapeHTML(cmd), escapeHTML(out))
	}

	b.updateShellMsg(chatID, s, body)
}

// finishShell 结束 Shell 会话：清除会话状态，并把终端消息恢复为容器管理面板菜单，
// 使这条消息回到进入 Shell 之前的形态，保持"单条消息流"。
func (b *Bot) finishShell(chatID int64, s *shellSession) {
	b.clearShell(chatID)
	// 退出后把这条消息重新编辑为容器面板菜单（复用 resultMsgID）
	b.sendContainerPanel(chatID, s.id, s.name, s.hostID, s.resultMsgID)
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
