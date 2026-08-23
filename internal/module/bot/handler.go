package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/compose"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
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
		b.handleCommand(chatID, strings.TrimSpace(u.Message.Text), u.Message.MessageID)
	}
}

// handleCommand 处理文本指令。msgID 为该文本消息的 ID（用于 Shell 会话删除命令消息）。
func (b *Bot) handleCommand(chatID int64, text string, msgID int64) {
	trimmed := strings.TrimSpace(text)
	// 最高优先级：若处于 Shell 会话中，除退出指令外的所有文本都作为命令执行。
	if s := b.getShell(chatID); s != nil {
		switch trimmed {
		case "/exit", "/quit", "/cancel":
			// 删除用户这条退出指令消息，保持对话干净；结果消息更新为已退出提示
			b.deleteMsg(chatID, msgID)
			b.finishShell(chatID, s)
		case "/menu", "/start":
			b.deleteMsg(chatID, msgID)
			b.clearShell(chatID)
			b.reply(chatID, fmt.Sprintf("已退出容器 <b>%s</b> 的 Shell 会话。", escapeHTML(s.name)))
			b.sendMainMenu(chatID)
		default:
			b.runShellCommand(chatID, s, trimmed, msgID)
		}
		return
	}
	// 其次处理等待输入的一次性会话动作（重命名/切标签）
	if p := b.takePending(chatID); p != nil {
		// /cancel 可取消待输入
		if trimmed == "/cancel" {
			b.reply(chatID, "已取消。")
			return
		}
		b.completePending(chatID, p, text)
		return
	}
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
		b.replyContainerList(chatID, false, 0, 0)
	case "/images":
		b.replyImageList(chatID, 0, 0)
	case "/sys":
		b.replySystemOverview(chatID, 0)
	case "/check_updates":
		b.checkAllUpdates(chatID)
	case "/update_all":
		b.updateAllContainers(chatID)
	case "/compose":
		b.listComposeProjects(chatID, 0, 0)
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
	var messageID int64 = 0
	if cb.Message != nil {
		messageID = cb.Message.MessageID
	}

	if cb.Data == "cancel" {
		if messageID > 0 {
			b.editMessage(chatID, messageID, "已取消。")
		} else {
			b.reply(chatID, "已取消。")
		}
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
			b.replyContainerList(chatID, false, page, messageID)
		case "run":
			b.replyContainerList(chatID, true, page, messageID)
		case "stopped":
			b.replyContainerListStopped(chatID, page, messageID)
		case "images":
			b.replyImageList(chatID, messageID, 0)
		case "sys":
			b.replySystemOverview(chatID, messageID)
		case "updates":
			// 更新中心：显示所有有更新的容器
			b.replyUpdateCenter(chatID, messageID, 0)
		case "backup":
			// 备份中心：显示备份管理面板
			b.replyBackupCenter(chatID, messageID)
		case "instance":
			// Docker 实例信息
			b.replyDockerInstance(chatID, messageID)
		case "settings":
			// 设置中心：整合多个设置入口
			b.replySettingsCenter(chatID, messageID)
		case "compose":
			b.listComposeProjects(chatID, messageID, 0)
		case "mute":
			// 更新通知设置面板
			b.sendMuteSettings(chatID, messageID)
		case "home":
			// 返回主菜单：编辑原消息为主菜单
			b.editMainMenu(chatID, messageID)
		case "help":
			// 帮助：编辑原消息展示，并带返回主菜单键盘，保持单条消息流
			backKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
			}}}
			b.editOrReplyKeyboard(chatID, messageID, helpText(), backKb)
		}
		return
	}
	// 更新通知屏蔽切换：mute|<容器名>
	if parts[0] == "mute" && len(parts) == 2 {
		b.toggleMute(chatID, parts[1], messageID)
		return
	}
	// 单容器操作面板：panel|<id>|<name>
	if parts[0] == "panel" && len(parts) == 3 {
		b.sendContainerPanel(chatID, parts[1], parts[2], messageID)
		return
	}
	// 面板子功能路由
	if len(parts) == 3 {
		id, name := parts[1], parts[2]
		switch parts[0] {
		case "logs":
			b.sendContainerLogs(chatID, id, name, messageID)
			return
		case "logdl":
			// 下载完整日志：作为文档发送
			b.sendContainerLogFile(chatID, id, name)
			return
		case "inspect":
			b.sendContainerInspect(chatID, id, name, messageID)
			return
		case "stats":
			b.sendContainerStats(chatID, id, name, messageID)
			return
		case "tags":
			b.sendTagSwitch(chatID, id, name, messageID)
			return
		case "execp":
			b.promptExec(chatID, id, name)
			return
		case "shexit":
			// 退出 Shell 会话按钮：若会话仍在则优雅结束（更新终端消息为已退出）
			if s := b.getShell(chatID); s != nil {
				b.finishShell(chatID, s)
			} else if messageID > 0 {
				b.editMessageKeyboard(chatID, messageID, fmt.Sprintf("✅ 容器 <b>%s</b> 的 Shell 会话已结束。", escapeHTML(name)), nil)
			}
			return
		case "shhist":
			// 查看历史命令按钮
			if s := b.getShell(chatID); s != nil {
				b.showShellHistory(chatID, s)
			} else if messageID > 0 {
				b.editMessage(chatID, messageID, "会话已结束，无法查看历史。")
			}
			return
		case "rename":
			b.promptRename(chatID, id, name)
			return
		case "tagin":
			b.promptTagInput(chatID, id, name)
			return
		}
	}
	// 切换 tag 执行：dotag|<id>|<name>|<tag>
	if parts[0] == "dotag" && len(parts) == 4 {
		b.doSwitchTag(chatID, parts[1], parts[2], parts[3], messageID)
		return
	}
	// 列表操作按钮：act|<action>|<id>|<name>
	if len(parts) == 4 && parts[0] == "act" {
		action := parts[1]
		// 低风险操作（启动/停止/重启/暂停/恢复/更新）直接执行；危险操作走二次确认
		switch action {
		case "start", "stop", "restart", "pause", "unpause", "update":
			b.execAction(chatID, action, parts[2], parts[3], messageID)
		default:
			b.askConfirm(chatID, action, parts[2], parts[3])
		}
		return
	}
	// 二次确认通过：confirm|<action>|<id>|<name>
	if len(parts) == 4 && parts[0] == "confirm" {
		b.execAction(chatID, parts[1], parts[2], parts[3], messageID)
		return
	}
	// 批量更新确认：batchupdate|confirm
	if parts[0] == "batchupdate" && len(parts) == 2 && parts[1] == "confirm" {
		b.executeBatchUpdate(chatID, messageID)
		return
	}
	// 镜像列表翻页：imgpg|<page>
	if parts[0] == "imgpg" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.replyImageList(chatID, messageID, page)
		return
	}
	// Compose 项目列表：cmpls（回到第 0 页）
	if parts[0] == "cmpls" {
		b.listComposeProjects(chatID, messageID, 0)
		return
	}
	// Compose 项目列表翻页：cmppg|<page>
	if parts[0] == "cmppg" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.listComposeProjects(chatID, messageID, page)
		return
	}
	// Compose 项目面板：cmpp|<projectID>
	if parts[0] == "cmpp" && len(parts) == 2 {
		b.showComposeProjectPanel(chatID, parts[1], messageID)
		return
	}
	// Compose 执行动作：cmpa|<projectID>|<action>
	if parts[0] == "cmpa" && len(parts) == 3 {
		b.executeComposeAction(chatID, parts[1], parts[2], messageID)
		return
	}
	// Compose 危险动作确认：cmpaconf|<projectID>|<action>
	if parts[0] == "cmpaconf" && len(parts) == 3 {
		// 重新扫描找到项目并执行
		scanPaths := b.svcCtx.Config.Compose.ScanPaths
		maxDepth := b.svcCtx.Config.Compose.MaxDepth
		scanner := compose.NewScanner(scanPaths, maxDepth)
		projects := scanner.Scan()
		var target *compose.Project
		for i := range projects {
			if projects[i].ID == parts[1] {
				target = &projects[i]
				break
			}
		}
		if target == nil {
			b.editOrReplyKeyboard(chatID, messageID, "❌ 项目不存在或已被移除", &telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{{
					{Text: "⬅ 返回项目列表", CallbackData: "cmpls"},
				}},
			})
			return
		}
		b.doComposeAction(chatID, target, parts[2], messageID)
		return
	}
	// 更新中心待更新列表翻页：updpg|<page>
	if parts[0] == "updpg" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.replyUpdateCenter(chatID, messageID, page)
		return
	}
	// 更新中心操作：updc|<action>
	if parts[0] == "updc" && len(parts) == 2 {
		switch parts[1] {
		case "check":
			// 重新检查更新
			b.recheckUpdates(chatID, messageID)
		}
		return
	}
	// 备份中心操作：backup|<action>
	if parts[0] == "backup" && len(parts) >= 2 {
		switch parts[1] {
		case "create":
			// 创建备份
			b.createBackup(chatID, messageID)
		case "list":
			// 查看备份列表
			b.listBackups(chatID, messageID)
		case "restore":
			// 恢复备份：backup|restore|<filename>
			if len(parts) == 3 {
				b.restoreBackup(chatID, parts[2], messageID)
			}
		case "delete":
			// 删除备份：backup|delete|<filename>
			if len(parts) == 3 {
				b.confirmDeleteBackup(chatID, parts[2], messageID)
			}
		case "delconf":
			// 确认删除：backup|delconf|<filename>
			if len(parts) == 3 {
				b.deleteBackup(chatID, parts[2], messageID)
			}
		}
		return
	}
	// 设置中心操作：settings|<action>
	if parts[0] == "settings" && len(parts) >= 2 {
		switch parts[1] {
		case "prune":
			// 清理镜像选择页面
			b.replyPruneImageOptions(chatID, messageID)
		case "prune_dangling":
			// 清理悬空镜像（二次确认）
			b.confirmPruneImages(chatID, "dangling", messageID)
		case "prune_unused":
			// 清理未使用镜像（二次确认）
			b.confirmPruneImages(chatID, "unused", messageID)
		case "prune_exec":
			// 执行清理：settings|prune_exec|<mode>
			if len(parts) == 3 {
				b.executePruneImages(chatID, parts[2], messageID)
			}
		}
		return
	}
	// 更新通知相关操作：notify|<action>
	if parts[0] == "notify" && len(parts) >= 2 {
		switch parts[1] {
		case "muteall":
			// 全部屏蔽通知（二次确认）
			b.confirmMuteAll(chatID, messageID)
		case "muteall_confirm":
			// 执行全部屏蔽
			b.executeMuteAll(chatID, messageID)
		case "interval":
			// 调整检查间隔
			b.replyUpdateInterval(chatID, messageID)
		case "setinterval":
			// 设置间隔：notify|setinterval|<hours>
			if len(parts) == 3 {
				b.setUpdateInterval(chatID, parts[2], messageID)
			}
		case "page":
			// 翻页：notify|page|<page>
			if len(parts) == 3 {
				page, _ := strconv.Atoi(parts[2])
				b.resendUpdateNotification(chatID, page, messageID)
			}
		}
		return
	}
}

// execAction 统一执行容器操作并回报结果，供直接点击与二次确认后调用。
// messageID > 0 时用于编辑原消息（特别是更新操作的进度显示）。
func (b *Bot) execAction(chatID int64, action, id, name string, messageID int64) {
	var err error
	switch action {
	case "start":
		err = b.ops.Start(id)
	case "stop":
		err = b.ops.Stop(id, 10)
	case "restart":
		err = b.ops.Restart(id, 10)
	case "pause":
		err = b.ops.Pause(id)
	case "unpause":
		err = b.ops.Unpause(id)
	case "kill":
		err = b.ops.Kill(id)
	case "remove":
		err = b.ops.Remove(id, true, false)
	case "update":
		b.doUpdate(chatID, id, name, messageID)
		return
	default:
		b.reply(chatID, "不支持的操作")
		return
	}
	// 删除类操作后容器已不存在，返回列表；其余操作返回该容器管理面板
	var backKb *telegram.InlineKeyboardMarkup
	if action == "remove" {
		backKb = &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "⬅ 返回列表", CallbackData: "menu|ps|0"},
		}}}
	} else {
		backKb = b.backToPanelKb(id, name)
	}
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID,
			fmt.Sprintf("❌ 容器 <b>%s</b> 执行「%s」失败：%s", escapeHTML(name), actionLabel(action), escapeHTML(err.Error())),
			backKb)
		return
	}
	b.editOrReplyKeyboard(chatID, messageID,
		fmt.Sprintf("✅ 容器 <b>%s</b> 已%s", escapeHTML(name), actionLabel(action)),
		backKb)
}

// findContainer 按 id 前缀查找容器，返回容器信息与是否找到。
func (b *Bot) findContainer(id string) (MyType.Container, bool) {
	list, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		return MyType.Container{}, false
	}
	list = utiles.CheckImageUpdate(b.svcCtx, list)
	for _, c := range list {
		if strings.HasPrefix(c.ID, id) {
			return c, true
		}
	}
	return MyType.Container{}, false
}

// sendContainerPanel 推送单容器操作面板：把该容器所有可用操作收纳到二级菜单，
// 避免列表页按钮爆炸。按钮随容器状态动态变化。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerPanel(chatID int64, id, name string, messageID int64) {
	c, ok := b.findContainer(id)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	running := strings.EqualFold(c.State, "running")
	paused := strings.EqualFold(c.State, "paused")

	// 面板文字：状态 + 镜像 + 更新标识
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>⚙ 容器管理：%s</b>\n", escapeHTML(name)))
	text.WriteString(fmt.Sprintf("状态：%s\n", stateLabel(c.State)))
	text.WriteString(fmt.Sprintf("镜像：<code>%s</code>\n", escapeHTML(shortImage(c.Image))))
	if c.Update {
		text.WriteString("🔺 有可用更新\n")
	}

	var rows [][]telegram.InlineKeyboardButton
	// 第一行：生命周期操作（随状态变化）
	var lifeRow []telegram.InlineKeyboardButton
	if running {
		lifeRow = append(lifeRow,
			telegram.InlineKeyboardButton{Text: "⏹ 停止", CallbackData: fmt.Sprintf("act|stop|%s|%s", id, name)},
			telegram.InlineKeyboardButton{Text: "🔄 重启", CallbackData: fmt.Sprintf("act|restart|%s|%s", id, name)},
			telegram.InlineKeyboardButton{Text: "⏸ 暂停", CallbackData: fmt.Sprintf("act|pause|%s|%s", id, name)},
		)
	} else if paused {
		lifeRow = append(lifeRow,
			telegram.InlineKeyboardButton{Text: "▶ 恢复", CallbackData: fmt.Sprintf("act|unpause|%s|%s", id, name)},
			telegram.InlineKeyboardButton{Text: "⏹ 停止", CallbackData: fmt.Sprintf("act|stop|%s|%s", id, name)},
		)
	} else {
		lifeRow = append(lifeRow,
			telegram.InlineKeyboardButton{Text: "▶ 启动", CallbackData: fmt.Sprintf("act|start|%s|%s", id, name)},
		)
	}
	rows = append(rows, lifeRow)

	// 第二行：更新 / 切换 tag
	updRow := []telegram.InlineKeyboardButton{
		{Text: "⬆ 更新", CallbackData: fmt.Sprintf("act|update|%s|%s", id, name)},
		{Text: "🏷 切换标签", CallbackData: fmt.Sprintf("tags|%s|%s", id, name)},
	}
	rows = append(rows, updRow)

	// 第三行：信息查看 + 命令行
	infoRow := []telegram.InlineKeyboardButton{
		{Text: "📄 日志", CallbackData: fmt.Sprintf("logs|%s|%s", id, name)},
		{Text: "🔍 详情", CallbackData: fmt.Sprintf("inspect|%s|%s", id, name)},
		{Text: "📊 资源", CallbackData: fmt.Sprintf("stats|%s|%s", id, name)},
		{Text: "💻 命令行", CallbackData: fmt.Sprintf("execp|%s|%s", id, name)},
	}
	rows = append(rows, infoRow)

	// 第四行：重命名
	opRow := []telegram.InlineKeyboardButton{
		{Text: "✏ 重命名", CallbackData: fmt.Sprintf("rename|%s|%s", id, name)},
	}
	rows = append(rows, opRow)

	// 第五行：危险操作（删除 / 强杀）
	dangerRow := []telegram.InlineKeyboardButton{
		{Text: "💀 强制终止", CallbackData: fmt.Sprintf("act|kill|%s|%s", id, name)},
		{Text: "🗑 删除", CallbackData: fmt.Sprintf("act|remove|%s|%s", id, name)},
	}
	rows = append(rows, dangerRow)

	// 第六行：返回列表
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回列表", CallbackData: "menu|ps|0"},
	})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// mainMenuKeyboard 返回主菜单按钮布局（发送与编辑共用）。
func mainMenuKeyboard() *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			// 第一行：概览 | 容器
			{{Text: "📊 概览", CallbackData: "menu|sys"}, {Text: "📦 容器", CallbackData: "menu|ps|0"}},
			// 第二行：更新 | 镜像
			{{Text: "🆙 更新", CallbackData: "menu|updates"}, {Text: "🐳 镜像", CallbackData: "menu|images"}},
			// 第三行：备份 | 实例
			{{Text: "📋 备份", CallbackData: "menu|backup"}, {Text: "💻 实例", CallbackData: "menu|instance"}},
			// 第四行：设置 | 帮助
			{{Text: "⚙️ 设置", CallbackData: "menu|settings"}, {Text: "📚 帮助", CallbackData: "menu|help"}},
		},
	}
}

// sendMainMenu 推送主菜单（按钮式交互入口）。
func (b *Bot) sendMainMenu(chatID int64) {
	b.replyKeyboard(chatID, "<b>DockerCopilot 控制台</b>\n请选择操作：", mainMenuKeyboard())
}

// editMainMenu 将现有消息编辑为主菜单（用于"返回主菜单"按钮）。
func (b *Bot) editMainMenu(chatID int64, messageID int64) {
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, "<b>DockerCopilot 控制台</b>\n请选择操作：", mainMenuKeyboard())
	} else {
		b.sendMainMenu(chatID)
	}
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
const pageSize = 10

// replyContainerList 分页推送容器列表，每个容器附带操作按钮。
// onlyRunning=true 时仅展示运行中容器；page 从 0 开始。
// messageID > 0 时编辑原消息（用于翻页），否则发送新消息。
func (b *Bot) replyContainerList(chatID int64, onlyRunning bool, page int, messageID int64) {
	list, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取容器列表失败："+err.Error())
		return
	}
	// 标记哪些容器有镜像更新（并发安全读取后台检查结果）
	list = utiles.CheckImageUpdate(b.svcCtx, list)
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
	mode := "ps"
	if onlyRunning {
		mode = "run"
	}

	// 一条消息：标题 + 带序号的容器文字列表
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>%s</b>（第 %d/%d 页，共 %d 个，点下方按钮操作）\n", title, page+1, totalPages, total))

	// 一整块内联键盘：每个容器一行「序号.管理」，点击进入单容器操作面板
	var rows [][]telegram.InlineKeyboardButton
	for i, c := range filtered[start:end] {
		seq := i + 1 // 页内序号 1..pageSize
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		// 更新标识
		updateFlag := ""
		if c.Update {
			updateFlag = " 🔺有更新"
		}
		// 文字列表：序号 + 汉化状态 + 名称 + 更新标识，第二行显示镜像:标签
		text.WriteString(fmt.Sprintf("\n<b>%d.</b> %s <b>%s</b>%s\n    <code>%s</code>",
			seq, stateLabel(c.State), escapeHTML(name), updateFlag, escapeHTML(shortImage(c.Image))))

		// 每行按钮：常用操作 + 更新（常驻）+ 「管理」进入面板
		var row []telegram.InlineKeyboardButton
		if strings.EqualFold(c.State, "running") {
			row = append(row,
				telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.⏹停止", seq), CallbackData: fmt.Sprintf("act|stop|%s|%s", id, name)},
				telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.🔄重启", seq), CallbackData: fmt.Sprintf("act|restart|%s|%s", id, name)},
			)
		} else {
			row = append(row,
				telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.▶启动", seq), CallbackData: fmt.Sprintf("act|start|%s|%s", id, name)},
			)
		}
		// 更新按钮常驻显示（有更新时显示提示标记）
		updateText := fmt.Sprintf("%d.⬆更新", seq)
		if c.Update {
			updateText = fmt.Sprintf("%d.⬆更新🔺", seq)
		}
		row = append(row, telegram.InlineKeyboardButton{Text: updateText, CallbackData: fmt.Sprintf("act|update|%s|%s", id, name)})
		row = append(row, telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.⚙管理", seq), CallbackData: fmt.Sprintf("panel|%s|%s", id, name)})
		rows = append(rows, row)
	}

	// 分页导航行：上一页 / 下一页
	var nav []telegram.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("menu|%s|%d", mode, page-1)})
	}
	if page < totalPages-1 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("menu|%s|%d", mode, page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}

	// 筛选按钮行：全部 | 运行中 | 已停止
	filterRow := []telegram.InlineKeyboardButton{
		{Text: "📦 全部", CallbackData: "menu|ps|0"},
		{Text: "🟢 运行中", CallbackData: "menu|run|0"},
		{Text: "⚪已停止", CallbackData: "menu|stopped|0"},
	}
	rows = append(rows, filterRow)

	// 返回主菜单按钮
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息（翻页场景）；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// imagesPageSize 镜像列表每页展示条数（避免单条消息过长）。
const imagesPageSize = 15

// replyImageList 分页推送镜像详细列表：每行显示 镜像名:标签 + 大小 + 是否在用。
// 按镜像大小倒序排列，大镜像在前方便用户识别存储占用大户。
// page 从 0 开始；messageID > 0 时编辑原消息（翻页），否则发送新消息。
func (b *Bot) replyImageList(chatID int64, messageID int64, page int) {
	list, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取镜像列表失败："+err.Error())
		return
	}
	if len(list) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "📦 当前没有任何镜像",
			&telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
			}}})
		return
	}

	// 按大小倒序排列（大镜像优先显示）
	sort.Slice(list, func(i, j int) bool {
		return list[i].Size > list[j].Size
	})

	// 分页计算：页码钳制到有效范围
	total := len(list)
	totalPages := (total + imagesPageSize - 1) / imagesPageSize
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * imagesPageSize
	end := start + imagesPageSize
	if end > total {
		end = total
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>📦 镜像列表（共 %d 个）</b>  第 %d/%d 页\n\n", total, page+1, totalPages))

	for _, img := range list[start:end] {
		// 镜像名:标签
		imageFull := img.ImageName + ":" + img.ImageTag
		if img.ImageName == "None" || img.ImageTag == "None" {
			imageFull = "&lt;none&gt;" // 无标签镜像
		}
		// 状态图标：是否在用
		statusIcon := "⚪"
		if img.InUsed {
			statusIcon = "🟢"
		}
		text.WriteString(fmt.Sprintf("%s <code>%s</code>\n", statusIcon, escapeHTML(imageFull)))
		text.WriteString(fmt.Sprintf("   📊 大小: %s", img.SizeFormat))
		if img.InUsed {
			text.WriteString(" | ✅ 使用中")
		} else {
			text.WriteString(" | ⚪ 未使用")
		}
		text.WriteString("\n\n")
	}
	text.WriteString("💡 <i>提示：🟢 使用中 | ⚪ 未使用</i>")

	// 翻页 + 返回键盘
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: buildPager("imgpg", page, totalPages, "menu|home")}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// replySystemOverview 推送系统资源概览：容器/镜像统计 + 磁盘占用汇总。
// 提供运行/停止容器数、镜像总数、使用中镜像数和总磁盘占用等关键指标。
// messageID > 0 时编辑原消息（用于从主菜单点击进入），否则发送新消息；
// 末尾统一附带「返回主菜单」按钮，保证菜单导航闭环。
func (b *Bot) replySystemOverview(chatID int64, messageID int64) {
	// 获取容器列表
	containers, errC := utiles.GetContainerList(b.svcCtx)
	if errC != nil {
		b.reply(chatID, "获取容器信息失败："+errC.Error())
		return
	}

	// 获取镜像列表
	images, errI := utiles.GetImagesList(b.svcCtx)
	if errI != nil {
		b.reply(chatID, "获取镜像信息失败："+errI.Error())
		return
	}

	// 统计容器状态
	totalContainers := len(containers)
	runningContainers := 0
	for _, c := range containers {
		if c.State == "running" {
			runningContainers++
		}
	}
	stoppedContainers := totalContainers - runningContainers

	// 统计镜像
	totalImages := len(images)
	usedImages := 0
	var totalDiskUsage int64
	for _, img := range images {
		totalDiskUsage += img.Size
		if img.InUsed {
			usedImages++
		}
	}
	unusedImages := totalImages - usedImages

	// 格式化磁盘占用
	diskUsageStr := ""
	if totalDiskUsage >= 1024*1024*1024 {
		diskUsageStr = fmt.Sprintf("%.2f GB", float64(totalDiskUsage)/1024/1024/1024)
	} else {
		diskUsageStr = fmt.Sprintf("%.2f MB", float64(totalDiskUsage)/1024/1024)
	}

	var text strings.Builder
	text.WriteString("<b>💻 系统概览</b>\n\n")

	text.WriteString("<b>📦 容器统计</b>\n")
	text.WriteString(fmt.Sprintf("   总数: %d\n", totalContainers))
	text.WriteString(fmt.Sprintf("   🟢 运行中: %d\n", runningContainers))
	text.WriteString(fmt.Sprintf("   ⚪ 已停止: %d\n\n", stoppedContainers))

	text.WriteString("<b>🖼 镜像统计</b>\n")
	text.WriteString(fmt.Sprintf("   总数: %d\n", totalImages))
	text.WriteString(fmt.Sprintf("   ✅ 使用中: %d\n", usedImages))
	text.WriteString(fmt.Sprintf("   ⚪ 未使用: %d\n\n", unusedImages))

	text.WriteString("<b>💾 磁盘占用</b>\n")
	text.WriteString(fmt.Sprintf("   镜像总占用: %s", diskUsageStr))

	// 附带返回主菜单按钮，保证从菜单进入后可原路返回
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
		}},
	}
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// checkAllUpdates 一键检查所有容器的镜像更新并回报结果。
// 触发后台检查流程，等待检查完成后展示有更新的容器列表。
func (b *Bot) checkAllUpdates(chatID int64) {
	b.reply(chatID, "🔍 正在检查所有容器的镜像更新，请稍候...")

	// 获取所有容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	if len(containers) == 0 {
		b.reply(chatID, "📦 当前没有任何容器")
		return
	}

	// 获取镜像列表并触发检查（CheckUpdate 方法会去重避免并发）
	images, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取镜像列表失败："+err.Error())
		return
	}

	// 触发后台更新检查
	b.svcCtx.HubImageInfo.CheckUpdate(images)

	// 等待检查完成（最多30秒）
	time.Sleep(2 * time.Second) // 给予初始检查时间

	// 重新获取容器列表并标记更新状态
	containers, _ = utiles.GetContainerList(b.svcCtx)
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	// 统计结果
	var needUpdateList []MyType.Container
	for _, c := range containers {
		if c.Update {
			needUpdateList = append(needUpdateList, c)
		}
	}

	var text strings.Builder
	text.WriteString("<b>🔍 镜像更新检查完成</b>\n\n")
	text.WriteString(fmt.Sprintf("总容器数: %d\n", len(containers)))
	text.WriteString(fmt.Sprintf("🔺 有可用更新: %d\n\n", len(needUpdateList)))

	if len(needUpdateList) > 0 {
		text.WriteString("<b>需要更新的容器：</b>\n")
		for i, c := range needUpdateList {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			if name == "" {
				name = c.ID[:12] // 使用短ID作为备选
			}
			text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n", i+1, escapeHTML(name), escapeHTML(shortImage(c.Image))))
		}
		text.WriteString("\n💡 使用 /update_all 批量更新所有容器")
	} else {
		text.WriteString("✅ 所有容器镜像均为最新版本")
	}

	// 附带操作键盘：有更新则给批量更新入口，始终带返回主菜单
	var rows [][]telegram.InlineKeyboardButton
	if len(needUpdateList) > 0 {
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: "✅ 批量更新全部", CallbackData: "batchupdate|confirm"},
		})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})
	b.replyKeyboard(chatID, text.String(), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

// updateAllContainers 批量更新所有有可用更新的容器（高风险操作，需二次确认）。
func (b *Bot) updateAllContainers(chatID int64) {
	// 获取所有容器并检查更新
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	// 筛选需要更新的容器
	var needUpdateList []MyType.Container
	for _, c := range containers {
		if c.Update {
			needUpdateList = append(needUpdateList, c)
		}
	}

	if len(needUpdateList) == 0 {
		b.reply(chatID, "✅ 没有容器需要更新")
		return
	}

	// 二次确认：列出将要更新的容器
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>⚠ 批量更新确认</b>\n\n将更新以下 %d 个容器：\n\n", len(needUpdateList)))
	for i, c := range needUpdateList {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			name = c.ID[:12] // 使用短ID作为备选
		}
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n", i+1, escapeHTML(name), escapeHTML(shortImage(c.Image))))
	}
	text.WriteString("\n⚠ <b>注意：批量更新会依次重建所有容器，过程中服务将短暂中断。</b>")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ 确认批量更新", CallbackData: "batchupdate|confirm"},
				{Text: "❌ 取消", CallbackData: "cancel"},
			},
		},
	}
	b.replyKeyboard(chatID, text.String(), kb)
}

// executeBatchUpdate 执行批量更新：依次为所有有更新的容器提交更新任务。
// messageID > 0 时把结果编辑到原消息（内联按钮触发），否则发新消息。
func (b *Bot) executeBatchUpdate(chatID int64, messageID int64) {
	backHomeKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	}}}
	// 获取所有需要更新的容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 获取容器列表失败："+escapeHTML(err.Error()), backHomeKb)
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	var needUpdateList []MyType.Container
	for _, c := range containers {
		if c.Update {
			needUpdateList = append(needUpdateList, c)
		}
	}

	if len(needUpdateList) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "✅ 没有容器需要更新", backHomeKb)
		return
	}

	// 执行前：编辑原消息为"开始批量更新"，不另发新消息
	b.editOrReplyKeyboard(chatID, messageID, fmt.Sprintf("🚀 开始批量更新 %d 个容器...\n", len(needUpdateList)), nil)

	// 依次提交更新任务
	successCount := 0
	var failedList []string
	for _, c := range needUpdateList {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			name = c.ID[:12] // 使用短ID作为备选
		}

		taskID, err := b.ops.Update(c.ID, name, c.Image)
		if err != nil {
			failedList = append(failedList, fmt.Sprintf("- %s: %s", name, err.Error()))
			continue
		}
		successCount++
		logx.Infof("批量更新：容器 %s 任务已提交，taskID=%s", name, taskID)
	}

	// 汇报结果
	var text strings.Builder
	text.WriteString("<b>📊 批量更新结果</b>\n\n")
	text.WriteString(fmt.Sprintf("✅ 成功提交: %d 个\n", successCount))
	if len(failedList) > 0 {
		text.WriteString(fmt.Sprintf("❌ 失败: %d 个\n\n", len(failedList)))
		text.WriteString("<b>失败详情：</b>\n")
		for _, fail := range failedList {
			text.WriteString(escapeHTML(fail) + "\n")
		}
	} else {
		text.WriteString("\n💡 所有更新任务已在后台执行，请稍后用 /ps 查看状态。")
	}

	// 结果编辑到原消息并带返回主菜单
	b.editOrReplyKeyboard(chatID, messageID, text.String(), backHomeKb)
}

// composePageSize Compose 项目列表每页展示条数（每项占一个按钮）。
const composePageSize = 8

// listComposeProjects 分页列出扫描到的 Compose 项目，每个项目一个按钮进入管理面板。
// page 从 0 开始；messageID > 0 时编辑原消息（翻页），否则发送新消息。
func (b *Bot) listComposeProjects(chatID int64, messageID int64, page int) {
	// 从配置获取扫描路径和深度
	scanPaths := b.svcCtx.Config.Compose.ScanPaths
	maxDepth := b.svcCtx.Config.Compose.MaxDepth
	// 返回主菜单按钮：错误提示与空列表也带上，避免用户卡在无按钮的独立消息里
	backHomeKb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
		}},
	}
	if len(scanPaths) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 未配置 Compose 项目扫描路径", backHomeKb)
		return
	}

	scanner := compose.NewScanner(scanPaths, maxDepth)
	projects := scanner.Scan()

	if len(projects) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "📂 未找到任何 Compose 项目\n\n请检查扫描路径配置。", backHomeKb)
		return
	}

	// 分页计算：页码钳制到有效范围
	total := len(projects)
	totalPages := (total + composePageSize - 1) / composePageSize
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * composePageSize
	end := start + composePageSize
	if end > total {
		end = total
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>🧩 Compose 项目列表（共 %d 个）</b>  第 %d/%d 页\n\n", total, page+1, totalPages))

	var rows [][]telegram.InlineKeyboardButton
	for i := start; i < end; i++ {
		proj := projects[i]
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n\n", i+1, escapeHTML(proj.Name), escapeHTML(proj.Dir)))
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d. ⚙ 管理 %s", i+1, proj.Name), CallbackData: fmt.Sprintf("cmpp|%s", proj.ID)},
		})
	}
	// 追加翻页行 + 返回主菜单
	for _, row := range buildPager("cmppg", page, totalPages, "menu|home") {
		rows = append(rows, row)
	}

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// showComposeProjectPanel 展示单个 Compose 项目的操作面板，提供 up/down/restart/pull/stop/start 按钮。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) showComposeProjectPanel(chatID int64, projectID string, messageID int64) {
	// 重新扫描找到该项目
	scanPaths := b.svcCtx.Config.Compose.ScanPaths
	maxDepth := b.svcCtx.Config.Compose.MaxDepth
	scanner := compose.NewScanner(scanPaths, maxDepth)
	projects := scanner.Scan()

	var target *compose.Project
	for i := range projects {
		if projects[i].ID == projectID {
			target = &projects[i]
			break
		}
	}
	if target == nil {
		// 项目不存在也带返回项目列表按钮，保证导航闭环
		b.editOrReplyKeyboard(chatID, messageID, "❌ 项目不存在或已被移除", &telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回项目列表", CallbackData: "cmpls"},
			}},
		})
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>🧩 Compose 项目：%s</b>\n\n", escapeHTML(target.Name)))
	text.WriteString(fmt.Sprintf("目录：<code>%s</code>\n", escapeHTML(target.Dir)))
	text.WriteString(fmt.Sprintf("主配置：<code>%s</code>\n\n", escapeHTML(target.ComposeFile)))
	text.WriteString("选择操作：")

	rows := [][]telegram.InlineKeyboardButton{
		{
			{Text: "🚀 up (启动)", CallbackData: fmt.Sprintf("cmpa|%s|up", projectID)},
			{Text: "🛑 down (停止并删除)", CallbackData: fmt.Sprintf("cmpa|%s|down", projectID)},
		},
		{
			{Text: "🔄 restart (重启)", CallbackData: fmt.Sprintf("cmpa|%s|restart", projectID)},
			{Text: "⬇ pull (拉取镜像)", CallbackData: fmt.Sprintf("cmpa|%s|pull", projectID)},
		},
		{
			{Text: "⏹ stop (停止)", CallbackData: fmt.Sprintf("cmpa|%s|stop", projectID)},
			{Text: "▶ start (启动)", CallbackData: fmt.Sprintf("cmpa|%s|start", projectID)},
		},
		{
			{Text: "⬅ 返回项目列表", CallbackData: "cmpls"},
		},
	}

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// executeComposeAction 执行 Compose 动作（危险操作如 down 需二次确认）。
func (b *Bot) executeComposeAction(chatID int64, projectID, action string, messageID int64) {
	// 重新扫描找到项目
	scanPaths := b.svcCtx.Config.Compose.ScanPaths
	maxDepth := b.svcCtx.Config.Compose.MaxDepth
	scanner := compose.NewScanner(scanPaths, maxDepth)
	projects := scanner.Scan()

	var target *compose.Project
	for i := range projects {
		if projects[i].ID == projectID {
			target = &projects[i]
			break
		}
	}
	if target == nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 项目不存在或已被移除", &telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回项目列表", CallbackData: "cmpls"},
			}},
		})
		return
	}

	// 危险操作二次确认（down 会删除容器和网络）
	if action == "down" {
		text := fmt.Sprintf("<b>⚠ 危险操作确认</b>\n\n项目：<b>%s</b>\n动作：<b>down (停止并删除所有容器和网络)</b>\n\n确定执行吗？", escapeHTML(target.Name))
		kb := &telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{
				{
					{Text: "✅ 确认执行", CallbackData: fmt.Sprintf("cmpaconf|%s|down", projectID)},
					{Text: "❌ 取消", CallbackData: fmt.Sprintf("cmpp|%s", projectID)},
				},
			},
		}
		b.editOrReplyKeyboard(chatID, messageID, text, kb)
		return
	}

	// 非危险操作直接执行
	b.doComposeAction(chatID, target, action, messageID)
}

// doComposeAction 实际执行 Compose 动作并回报结果。
// messageID > 0 时先把「正在执行」编辑到原消息，执行完再编辑为结果，保持单条消息流。
func (b *Bot) doComposeAction(chatID int64, proj *compose.Project, action string, messageID int64) {
	if !compose.IsSupportedAction(action) {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 不支持的操作："+action, &telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回项目管理", CallbackData: fmt.Sprintf("cmpp|%s", proj.ID)},
			}},
		})
		return
	}

	actionLabel := map[string]string{
		"up":      "启动",
		"down":    "停止并删除",
		"restart": "重启",
		"pull":    "拉取镜像",
		"stop":    "停止",
		"start":   "启动",
	}[action]

	// 执行前：编辑原消息为"正在执行"，不再另发新消息刷屏
	b.editOrReplyKeyboard(chatID, messageID, fmt.Sprintf("🚀 正在执行 <b>%s</b> - %s...", escapeHTML(proj.Name), actionLabel), nil)

	// 执行 Compose 命令（超时 5 分钟）
	ctx := context.Background()
	result := compose.RunAction(ctx, proj.Dir, proj.ComposeFile, action, 300)

	var text strings.Builder
	if result.Success {
		text.WriteString(fmt.Sprintf("✅ <b>%s</b> - %s 成功\n\n", escapeHTML(proj.Name), actionLabel))
	} else {
		text.WriteString(fmt.Sprintf("❌ <b>%s</b> - %s 失败\n\n", escapeHTML(proj.Name), actionLabel))
	}

	// 输出内容（限制长度避免超 TG 上限）
	output := strings.TrimSpace(result.Output)
	if output != "" {
		if len(output) > 2000 {
			output = output[len(output)-2000:]
			text.WriteString("<b>输出（最后 2000 字符）：</b>\n<pre>")
		} else {
			text.WriteString("<b>输出：</b>\n<pre>")
		}
		text.WriteString(escapeHTML(output))
		text.WriteString("</pre>\n\n")
	}

	text.WriteString(fmt.Sprintf("⏱ 耗时：%d ms", result.Duration))

	// 返回项目面板按钮
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "⬅ 返回项目管理", CallbackData: fmt.Sprintf("cmpp|%s", proj.ID)}},
		},
	}
	// 编辑原消息展示结果（与执行前的"正在执行"同一条消息）
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// ========== 消息编辑辅助函数 ==========

// editMessage 编辑已发送的消息文本（用于 callback 交互优化）。
func (b *Bot) editMessage(chatID int64, messageID int64, text string) {
	if err := b.client.EditMessageText(chatID, messageID, text, nil); err != nil {
		logx.Errorf("编辑消息失败 (chat=%d, msg=%d): %v", chatID, messageID, err)
	}
}

// editMessageKeyboard 编辑已发送的消息文本和键盘（用于列表翻页、面板切换等）。
func (b *Bot) editMessageKeyboard(chatID int64, messageID int64, text string, keyboard *telegram.InlineKeyboardMarkup) {
	if err := b.client.EditMessageText(chatID, messageID, text, keyboard); err != nil {
		logx.Errorf("编辑消息键盘失败 (chat=%d, msg=%d): %v", chatID, messageID, err)
	}
}

// editOrReplyKeyboard 统一封装「有 messageID 则编辑原消息、否则发新消息」的带键盘发送逻辑，
// 避免各处重复 if messageID > 0 的样板代码。
func (b *Bot) editOrReplyKeyboard(chatID int64, messageID int64, text string, keyboard *telegram.InlineKeyboardMarkup) {
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, keyboard)
	} else {
		b.replyKeyboard(chatID, text, keyboard)
	}
}

// ========== 新增主菜单功能 ==========

// updateCenterPageSize 更新中心"待更新容器"列表每页展示条数。
const updateCenterPageSize = 20

// replyUpdateCenter 更新中心：分页显示所有有可用更新的容器列表，提供一键检查和批量更新。
// page 从 0 开始（仅对待更新列表分页）；messageID > 0 时编辑原消息。
func (b *Bot) replyUpdateCenter(chatID int64, messageID int64, page int) {
	// 获取所有容器并检查更新状态
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 获取容器列表失败："+escapeHTML(err.Error()),
			&telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
			}}})
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	// 筛选需要更新的容器
	var needUpdateList []MyType.Container
	for _, c := range containers {
		if c.Update {
			needUpdateList = append(needUpdateList, c)
		}
	}

	// 分页计算（仅对待更新列表）
	total := len(needUpdateList)
	totalPages := 1
	if total > 0 {
		totalPages = (total + updateCenterPageSize - 1) / updateCenterPageSize
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	var text strings.Builder
	text.WriteString("<b>🆙 更新中心</b>\n\n")
	text.WriteString(fmt.Sprintf("总容器数: %d\n", len(containers)))
	text.WriteString(fmt.Sprintf("🔺 有可用更新: %d\n\n", total))

	if total > 0 {
		start := page * updateCenterPageSize
		end := start + updateCenterPageSize
		if end > total {
			end = total
		}
		text.WriteString(fmt.Sprintf("<b>需要更新的容器（第 %d/%d 页）：</b>\n", page+1, totalPages))
		for i := start; i < end; i++ {
			c := needUpdateList[i]
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			if name == "" {
				name = c.ID[:12]
			}
			text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n", i+1, escapeHTML(name), escapeHTML(shortImage(c.Image))))
		}
	} else {
		text.WriteString("✅ 所有容器镜像均为最新版本")
	}

	// 构建按钮：翻页行（仅多页时）+ 检查/批量更新 + 返回
	var rows [][]telegram.InlineKeyboardButton
	if total > 0 && totalPages > 1 {
		var nav []telegram.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("updpg|%d", page-1)})
		}
		nav = append(nav, telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d/%d", page+1, totalPages), CallbackData: fmt.Sprintf("updpg|%d", page)})
		if page < totalPages-1 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("updpg|%d", page+1)})
		}
		rows = append(rows, nav)
	}
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "🔍 重新检查更新", CallbackData: "updc|check"},
	})
	if total > 0 {
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: "✅ 批量更新全部", CallbackData: "batchupdate|confirm"},
		})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})

	b.editOrReplyKeyboard(chatID, messageID, text.String(), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

// replyBackupCenter 备份中心：显示备份管理面板。
func (b *Bot) replyBackupCenter(chatID int64, messageID int64) {
	var text strings.Builder
	text.WriteString("<b>📋 备份中心</b>\n\n")
	text.WriteString("容器备份功能允许您保存和恢复容器配置。\n\n")
	text.WriteString("<b>可用操作：</b>\n")
	text.WriteString("• 创建备份：导出所有容器配置为 JSON\n")
	text.WriteString("• 查看备份：浏览已有备份列表\n")
	text.WriteString("• 恢复容器：从备份恢复容器配置\n")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "📦 创建新备份", CallbackData: "backup|create"}},
			{{Text: "📂 查看备份列表", CallbackData: "backup|list"}},
			{{Text: "⬅ 返回主菜单", CallbackData: "menu|home"}},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// replyDockerInstance Docker 实例信息：显示当前 Docker 守护进程的详细信息。
func (b *Bot) replyDockerInstance(chatID int64, messageID int64) {
	// 获取 Docker 信息
	info, err := b.svcCtx.DockerClient.Info(context.Background())
	if err != nil {
		text := "❌ 获取 Docker 实例信息失败：" + err.Error()
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	var text strings.Builder
	text.WriteString("<b>💻 Docker 实例信息</b>\n\n")

	text.WriteString("<b>基本信息</b>\n")
	text.WriteString(fmt.Sprintf("名称: %s\n", escapeHTML(info.Name)))
	text.WriteString(fmt.Sprintf("版本: %s\n", escapeHTML(info.ServerVersion)))
	text.WriteString(fmt.Sprintf("操作系统: %s\n", escapeHTML(info.OperatingSystem)))
	text.WriteString(fmt.Sprintf("架构: %s\n\n", escapeHTML(info.Architecture)))

	text.WriteString("<b>资源状态</b>\n")
	text.WriteString(fmt.Sprintf("容器数: %d (运行: %d)\n", info.Containers, info.ContainersRunning))
	text.WriteString(fmt.Sprintf("镜像数: %d\n", info.Images))
	text.WriteString(fmt.Sprintf("CPU 核心: %d\n", info.NCPU))
	text.WriteString(fmt.Sprintf("内存: %.2f GB\n\n", float64(info.MemTotal)/1024/1024/1024))

	text.WriteString("<b>存储驱动</b>\n")
	text.WriteString(fmt.Sprintf("驱动: %s\n", escapeHTML(info.Driver)))

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "⬅ 返回主菜单", CallbackData: "menu|home"}},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// replySettingsCenter 设置中心：整合多个设置入口。
func (b *Bot) replySettingsCenter(chatID int64, messageID int64) {
	var text strings.Builder
	text.WriteString("<b>⚙️ 设置中心</b>\n\n")
	text.WriteString("请选择要配置的功能：")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🔕 更新通知设置", CallbackData: "menu|mute"}},
			{{Text: "🧩 Compose 项目", CallbackData: "menu|compose"}},
			{{Text: "🗑️ 清理未使用镜像", CallbackData: "settings|prune"}},
			{{Text: "⬅ 返回主菜单", CallbackData: "menu|home"}},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// replyContainerListStopped 显示已停止的容器列表（过滤掉运行中的容器）。
func (b *Bot) replyContainerListStopped(chatID int64, page int, messageID int64) {
	list, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取容器列表失败："+err.Error())
		return
	}
	// 标记哪些容器有镜像更新
	list = utiles.CheckImageUpdate(b.svcCtx, list)

	// 过滤：仅显示已停止的容器
	filtered := list[:0:0]
	for _, c := range list {
		if !strings.EqualFold(c.State, "running") {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		text := "⚪ 没有已停止的容器"
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
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

	// 构建消息
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>已停止容器</b>（第 %d/%d 页，共 %d 个）\n", page+1, totalPages, total))

	// 容器列表
	var rows [][]telegram.InlineKeyboardButton
	for i, c := range filtered[start:end] {
		seq := i + 1
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		updateFlag := ""
		if c.Update {
			updateFlag = " 🔺有更新"
		}
		text.WriteString(fmt.Sprintf("\n<b>%d.</b> %s <b>%s</b>%s\n    <code>%s</code>",
			seq, stateLabel(c.State), escapeHTML(name), updateFlag, escapeHTML(shortImage(c.Image))))

		// 按钮：启动 + 更新 + 管理
		row := []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d.▶启动", seq), CallbackData: fmt.Sprintf("act|start|%s|%s", id, name)},
		}
		updateText := fmt.Sprintf("%d.⬆更新", seq)
		if c.Update {
			updateText = fmt.Sprintf("%d.⬆更新🔺", seq)
		}
		row = append(row,
			telegram.InlineKeyboardButton{Text: updateText, CallbackData: fmt.Sprintf("act|update|%s|%s", id, name)},
			telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.⚙管理", seq), CallbackData: fmt.Sprintf("panel|%s|%s", id, name)},
		)
		rows = append(rows, row)
	}

	// 分页导航
	var nav []telegram.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("menu|stopped|%d", page-1)})
	}
	if page < totalPages-1 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("menu|stopped|%d", page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}

	// 筛选按钮
	filterRow := []telegram.InlineKeyboardButton{
		{Text: "📦 全部", CallbackData: "menu|ps|0"},
		{Text: "🟢 运行中", CallbackData: "menu|run|0"},
		{Text: "⚪已停止", CallbackData: "menu|stopped|0"},
	}
	rows = append(rows, filterRow)

	// 返回主菜单
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// recheckUpdates 重新检查所有容器的镜像更新并刷新更新中心。
func (b *Bot) recheckUpdates(chatID int64, messageID int64) {
	// 先发送检查中的提示
	if messageID > 0 {
		b.editMessage(chatID, messageID, "🔍 正在检查所有容器的镜像更新，请稍候...")
	} else {
		b.reply(chatID, "🔍 正在检查所有容器的镜像更新，请稍候...")
	}

	// 获取镜像列表并触发检查
	images, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取镜像列表失败："+err.Error())
		return
	}

	// 触发后台更新检查（CheckUpdate 会自动去重）
	b.svcCtx.HubImageInfo.CheckUpdate(images)

	// 等待检查完成
	time.Sleep(2 * time.Second)

	// 刷新更新中心页面
	b.replyUpdateCenter(chatID, messageID, 0)
}

// createBackup 创建容器配置备份。
func (b *Bot) createBackup(chatID int64, messageID int64) {
	// 发送处理中提示
	if messageID > 0 {
		b.editMessage(chatID, messageID, "📦 正在创建备份，请稍候...")
	} else {
		b.reply(chatID, "📦 正在创建备份，请稍候...")
	}

	// 调用备份函数
	err := utiles.BackupContainer(b.svcCtx)
	if err != nil {
		text := "❌ 备份创建失败：" + err.Error()
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	// 备份成功：直接把原消息编辑为最新备份列表（不再另发新消息）
	b.listBackups(chatID, messageID)
}

// listBackups 显示备份文件列表。
func (b *Bot) listBackups(chatID int64, messageID int64) {
	backupList, err := utiles.BackupList(b.svcCtx)
	if err != nil {
		text := "❌ 获取备份列表失败：" + err.Error()
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	var text strings.Builder
	text.WriteString("<b>📂 备份列表</b>\n\n")

	if len(backupList) == 0 {
		text.WriteString("暂无备份文件\n\n")
		text.WriteString("💡 点击下方按钮创建第一个备份")
	} else {
		text.WriteString(fmt.Sprintf("共 %d 个备份文件：\n\n", len(backupList)))
		for i, filename := range backupList {
			text.WriteString(fmt.Sprintf("%d. <code>%s</code>\n", i+1, escapeHTML(filename)))
		}
	}

	// 构建按钮
	var rows [][]telegram.InlineKeyboardButton

	// 为每个备份文件创建操作按钮（最多显示前10个）
	maxDisplay := 10
	if len(backupList) > maxDisplay {
		backupList = backupList[:maxDisplay]
	}

	for i, filename := range backupList {
		row := []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d.📥 恢复", i+1), CallbackData: fmt.Sprintf("backup|restore|%s", filename)},
			{Text: fmt.Sprintf("%d.🗑️ 删除", i+1), CallbackData: fmt.Sprintf("backup|delete|%s", filename)},
		}
		rows = append(rows, row)
	}

	// 底部功能按钮
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "📦 创建新备份", CallbackData: "backup|create"},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回备份中心", CallbackData: "menu|backup"},
	})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// restoreBackup 从备份恢复容器（提交后台任务）。
// messageID > 0 时把进度/结果编辑到原消息，保持单条消息流。
func (b *Bot) restoreBackup(chatID int64, filename string, messageID int64) {
	backKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回备份列表", CallbackData: "backup|list"},
	}}}
	// 生成任务ID
	taskID := fmt.Sprintf("restore_%d_%d", chatID, time.Now().Unix())

	// 执行前：编辑原消息为"正在恢复"
	b.editOrReplyKeyboard(chatID, messageID,
		fmt.Sprintf("🔄 正在从备份 <code>%s</code> 恢复容器...\n\n任务ID: <code>%s</code>\n请稍候，完成后会更新结果。", escapeHTML(filename), taskID), nil)

	// 提交恢复任务，完成后编辑原消息为结果
	go func() {
		err := utiles.RestoreContainer(b.svcCtx, filename, taskID)
		if err != nil {
			b.editOrReplyKeyboard(chatID, messageID,
				fmt.Sprintf("❌ 恢复备份 <code>%s</code> 失败：%s", escapeHTML(filename), escapeHTML(err.Error())), backKb)
		} else {
			b.editOrReplyKeyboard(chatID, messageID,
				fmt.Sprintf("✅ 备份 <code>%s</code> 恢复成功", escapeHTML(filename)), backKb)
		}
	}()
}

// confirmDeleteBackup 确认删除备份（二次确认）。
func (b *Bot) confirmDeleteBackup(chatID int64, filename string, messageID int64) {
	text := fmt.Sprintf("⚠️ <b>确认删除备份</b>\n\n即将删除备份文件：\n<code>%s</code>\n\n此操作不可恢复，确定要删除吗？", escapeHTML(filename))

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ 确认删除", CallbackData: fmt.Sprintf("backup|delconf|%s", filename)},
				{Text: "❌ 取消", CallbackData: "backup|list"},
			},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// deleteBackup 删除备份文件。
func (b *Bot) deleteBackup(chatID int64, filename string, messageID int64) {
	// 获取备份目录
	backupDir := os.Getenv("BACKUP_DIR")
	if backupDir == "" {
		backupDir = "/data/backups"
	}

	fullPath := filepath.Join(backupDir, filename)

	// 删除文件
	err := os.Remove(fullPath)
	if err != nil {
		text := fmt.Sprintf("❌ 删除备份失败：%s", err.Error())
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	// 删除成功：直接把原消息编辑为刷新后的备份列表（不再另发新消息）
	b.listBackups(chatID, messageID)
}

// replyPruneImageOptions 清理镜像选择页面：让用户选择清理范围。
func (b *Bot) replyPruneImageOptions(chatID int64, messageID int64) {
	var text strings.Builder
	text.WriteString("<b>🗑️ 清理未使用镜像</b>\n\n")
	text.WriteString("请选择清理范围：\n\n")
	text.WriteString("<b>1. 悬空镜像（Dangling）</b>\n")
	text.WriteString("仅清理无标签的镜像（&lt;none&gt;），较为安全。\n\n")
	text.WriteString("<b>2. 未使用镜像（Unused）</b>\n")
	text.WriteString("清理所有未被任何容器使用的镜像，包括有标签但未使用的镜像。\n")
	text.WriteString("⚠️ 此操作可能会删除暂时不用但以后需要的镜像。")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🗑️ 清理悬空镜像", CallbackData: "settings|prune_dangling"}},
			{{Text: "⚠️ 清理未使用镜像", CallbackData: "settings|prune_unused"}},
			{{Text: "⬅ 返回设置中心", CallbackData: "menu|settings"}},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// confirmPruneImages 确认清理镜像（二次确认，显示将要清理的镜像列表）。
func (b *Bot) confirmPruneImages(chatID int64, mode string, messageID int64) {
	// 获取镜像列表
	images, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		text := "❌ 获取镜像列表失败：" + err.Error()
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	// 筛选待清理的镜像
	var targetImages []MyType.Image
	for _, img := range images {
		if mode == "dangling" {
			// 悬空镜像：无标签（<none>:<none>）
			if img.ImageName == "<none>" && img.ImageTag == "<none>" {
				targetImages = append(targetImages, img)
			}
		} else if mode == "unused" {
			// 未使用镜像：不在使用中
			if !img.InUsed {
				targetImages = append(targetImages, img)
			}
		}
	}

	if len(targetImages) == 0 {
		modeText := "悬空镜像"
		if mode == "unused" {
			modeText = "未使用镜像"
		}
		text := fmt.Sprintf("✅ 没有需要清理的%s", modeText)
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	// 构建确认消息
	var text strings.Builder
	modeText := "悬空镜像"
	if mode == "unused" {
		modeText = "未使用镜像"
	}
	text.WriteString(fmt.Sprintf("<b>⚠️ 确认清理%s</b>\n\n", modeText))
	text.WriteString(fmt.Sprintf("将清理以下 %d 个镜像：\n\n", len(targetImages)))

	// 显示前10个
	maxDisplay := 10
	for i, img := range targetImages {
		if i >= maxDisplay {
			text.WriteString(fmt.Sprintf("\n... 还有 %d 个镜像", len(targetImages)-maxDisplay))
			break
		}
		imageName := fmt.Sprintf("%s:%s", img.ImageName, img.ImageTag)
		sizeStr := ""
		if img.Size >= 1024*1024*1024 {
			sizeStr = fmt.Sprintf("%.2f GB", float64(img.Size)/1024/1024/1024)
		} else {
			sizeStr = fmt.Sprintf("%.2f MB", float64(img.Size)/1024/1024)
		}
		text.WriteString(fmt.Sprintf("%d. <code>%s</code> (%s)\n", i+1, escapeHTML(imageName), sizeStr))
	}

	text.WriteString("\n⚠️ <b>此操作不可恢复，确定要清理吗？</b>")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ 确认清理", CallbackData: fmt.Sprintf("settings|prune_exec|%s", mode)},
				{Text: "❌ 取消", CallbackData: "settings|prune"},
			},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// executePruneImages 执行清理镜像（提交后台任务）。
// messageID > 0 时把"正在清理"编辑到原消息，完成后编辑为结果并带返回设置键盘。
func (b *Bot) executePruneImages(chatID int64, mode string, messageID int64) {
	// 返回设置中心的键盘
	backKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回设置", CallbackData: "menu|settings"},
	}}}
	// 获取镜像列表
	images, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 获取镜像列表失败："+escapeHTML(err.Error()), backKb)
		return
	}

	// 筛选待清理的镜像ID
	var imageIDs []string
	for _, img := range images {
		if mode == "dangling" {
			if img.ImageName == "<none>" && img.ImageTag == "<none>" {
				imageIDs = append(imageIDs, img.ID)
			}
		} else if mode == "unused" {
			if !img.InUsed {
				imageIDs = append(imageIDs, img.ID)
			}
		}
	}

	if len(imageIDs) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "✅ 没有需要清理的镜像", backKb)
		return
	}

	// 生成任务ID
	taskID := fmt.Sprintf("prune_%d_%d", chatID, time.Now().Unix())

	modeText := "悬空镜像"
	if mode == "unused" {
		modeText = "未使用镜像"
	}
	// 执行前：编辑原消息为"正在清理"
	b.editOrReplyKeyboard(chatID, messageID,
		fmt.Sprintf("🔄 正在清理%s...\n\n任务ID: <code>%s</code>\n共 %d 个镜像\n请稍候，完成后会更新结果。", modeText, taskID, len(imageIDs)), nil)

	// 提交清理任务
	go func() {
		taskCtx := context.Background()
		utiles.PruneImages(taskCtx, b.svcCtx, taskID, imageIDs, false)
		// 完成后编辑原消息为结果
		b.editOrReplyKeyboard(chatID, messageID,
			fmt.Sprintf("🗑 镜像清理任务已完成\n\n任务ID: <code>%s</code>\n共清理 %d 个镜像", taskID, len(imageIDs)), backKb)
	}()
}

// sendUpdateNotificationToChat 向指定会话发送带交互式键盘的更新通知（支持分页）。
func (b *Bot) sendUpdateNotificationToChat(chatID int64, containers []UpdateContainer) {
	b.sendUpdateNotificationToChatWithPage(chatID, containers, 0, 0)
}

// sendUpdateNotificationToChatWithPage 向指定会话发送带分页的更新通知。
func (b *Bot) sendUpdateNotificationToChatWithPage(chatID int64, containers []UpdateContainer, page int, messageID int64) {
	if len(containers) == 0 {
		return
	}

	// 每页最多10个容器
	pageSize := 10
	totalPages := (len(containers) + pageSize - 1) / pageSize

	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > len(containers) {
		end = len(containers)
	}

	var text strings.Builder
	text.WriteString("<b>🔔 容器更新提醒</b>\n\n")
	text.WriteString(fmt.Sprintf("检测到 %d 个容器有可用更新", len(containers)))
	if totalPages > 1 {
		text.WriteString(fmt.Sprintf("（第 %d/%d 页）", page+1, totalPages))
	}
	text.WriteString("：\n\n")

	// 构建内联键盘
	var rows [][]telegram.InlineKeyboardButton

	// 当前页的容器列表
	pageContainers := containers[start:end]
	for i, c := range pageContainers {
		seq := start + i + 1
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n", seq, escapeHTML(c.Name), escapeHTML(shortImage(c.Image))))

		row := []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d.更新", seq), CallbackData: fmt.Sprintf("act|update|%s|%s", c.ID, c.Name)},
			{Text: fmt.Sprintf("%d.屏蔽通知", seq), CallbackData: fmt.Sprintf("mute|%s", c.Name)},
		}
		rows = append(rows, row)
	}

	// 分隔线（视觉区分）
	text.WriteString("\n" + strings.Repeat("━", 30) + "\n")

	// 分页导航按钮（如果需要）
	if totalPages > 1 {
		var navRow []telegram.InlineKeyboardButton
		if page > 0 {
			navRow = append(navRow, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("notify|page|%d", page-1)})
		}
		if page < totalPages-1 {
			navRow = append(navRow, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("notify|page|%d", page+1)})
		}
		if len(navRow) > 0 {
			rows = append(rows, navRow)
		}
	}

	// 底部批量操作按钮
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "✅ 全部更新", CallbackData: "batchupdate|confirm"},
		{Text: "🔕 全部屏蔽", CallbackData: "notify|muteall"},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⚙️ 调整检查间隔", CallbackData: "notify|interval"},
	})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// confirmMuteAll 确认全部屏蔽更新通知（二次确认）。
func (b *Bot) confirmMuteAll(chatID int64, messageID int64) {
	// 获取所有有更新的容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	var needUpdateList []string
	for _, c := range containers {
		if c.Update {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			if name != "" {
				needUpdateList = append(needUpdateList, name)
			}
		}
	}

	if len(needUpdateList) == 0 {
		text := "✅ 当前没有需要更新的容器"
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	text := fmt.Sprintf("⚠️ <b>确认屏蔽全部更新通知</b>\n\n将屏蔽以下 %d 个容器的更新通知：\n\n", len(needUpdateList))
	for i, name := range needUpdateList {
		if i >= 10 {
			text += fmt.Sprintf("\n... 还有 %d 个容器", len(needUpdateList)-10)
			break
		}
		text += fmt.Sprintf("%d. <code>%s</code>\n", i+1, escapeHTML(name))
	}
	text += "\n确定要全部屏蔽吗？"

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ 确认屏蔽", CallbackData: "notify|muteall_confirm"},
				{Text: "❌ 取消", CallbackData: "cancel"},
			},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// executeMuteAll 执行全部屏蔽更新通知。
func (b *Bot) executeMuteAll(chatID int64, messageID int64) {
	// 获取所有有更新的容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	var mutedList []string
	cfg := b.svcCtx.AppConfig.Get()
	mutedSet := make(map[string]struct{})
	for _, m := range cfg.Telegram.MutedContainers {
		mutedSet[m] = struct{}{}
	}

	for _, c := range containers {
		if c.Update {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			if name != "" {
				if _, exists := mutedSet[name]; !exists {
					mutedSet[name] = struct{}{}
					mutedList = append(mutedList, name)
				}
			}
		}
	}

	if len(mutedList) == 0 {
		b.reply(chatID, "✅ 所有容器已经被屏蔽")
		return
	}

	// 更新配置：合并所有屏蔽项后经 AppConfig.Update 事务持久化
	newMuteList := make([]string, 0, len(mutedSet))
	for name := range mutedSet {
		newMuteList = append(newMuteList, name)
	}
	if err := b.svcCtx.AppConfig.Update(func(c *appconfig.AppConfig) error {
		c.Telegram.MutedContainers = newMuteList
		return nil
	}); err != nil {
		b.reply(chatID, "❌ 保存配置失败："+err.Error())
		return
	}

	b.reply(chatID, fmt.Sprintf("✅ 已屏蔽 %d 个容器的更新通知", len(mutedList)))
}

// replyUpdateInterval 显示调整检查间隔的选项。
func (b *Bot) replyUpdateInterval(chatID int64, messageID int64) {
	cfg := b.svcCtx.AppConfig.Get()
	// 配置以分钟存储(UpdateCheckIntervalMinutes)，展示时换算为小时时长字符串
	minutes := cfg.Telegram.UpdateCheckIntervalMinutes
	if minutes <= 0 {
		minutes = 24 * 60
	}
	currentInterval := (time.Duration(minutes) * time.Minute).String()

	var text strings.Builder
	text.WriteString("<b>⚙️ 调整更新检查间隔</b>\n\n")
	text.WriteString(fmt.Sprintf("当前检查间隔：<code>%s</code>\n\n", escapeHTML(currentInterval)))
	text.WriteString("请选择新的检查间隔：")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "1 小时", CallbackData: "notify|setinterval|1h"}, {Text: "3 小时", CallbackData: "notify|setinterval|3h"}},
			{{Text: "6 小时", CallbackData: "notify|setinterval|6h"}, {Text: "12 小时", CallbackData: "notify|setinterval|12h"}},
			{{Text: "24 小时", CallbackData: "notify|setinterval|24h"}, {Text: "48 小时", CallbackData: "notify|setinterval|48h"}},
			{{Text: "⬅ 返回", CallbackData: "cancel"}},
		},
	}

	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// setUpdateInterval 设置更新检查间隔。
func (b *Bot) setUpdateInterval(chatID int64, interval string, messageID int64) {
	// 验证间隔格式
	if _, err := time.ParseDuration(interval); err != nil {
		b.reply(chatID, "❌ 无效的时间间隔格式")
		return
	}

	// 将时长字符串(如 24h)换算为分钟后持久化到 UpdateCheckIntervalMinutes
	dur, _ := time.ParseDuration(interval)
	minutes := int(dur.Minutes())
	if err := b.svcCtx.AppConfig.Update(func(c *appconfig.AppConfig) error {
		c.Telegram.UpdateCheckIntervalMinutes = minutes
		return nil
	}); err != nil {
		text := "❌ 保存配置失败：" + err.Error()
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	text := fmt.Sprintf("✅ 更新检查间隔已设置为 <code>%s</code>\n\n新间隔将在下次检查时生效。", escapeHTML(interval))
	if messageID > 0 {
		b.editMessage(chatID, messageID, text)
	} else {
		b.reply(chatID, text)
	}
}

// resendUpdateNotification 重新获取更新列表并发送通知（用于翻页）。
func (b *Bot) resendUpdateNotification(chatID int64, page int, messageID int64) {
	// 获取所有有更新的容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	// 筛选有更新的容器
	cfg := b.svcCtx.AppConfig.Get()
	mutedSet := make(map[string]struct{})
	for _, m := range cfg.Telegram.MutedContainers {
		mutedSet[m] = struct{}{}
	}

	var updateContainers []UpdateContainer
	for _, c := range containers {
		if !c.Update {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			continue
		}
		// 跳过已屏蔽的容器
		if _, muted := mutedSet[name]; muted {
			continue
		}
		updateContainers = append(updateContainers, UpdateContainer{
			ID:    c.ID,
			Name:  name,
			Image: c.Image,
		})
	}

	if len(updateContainers) == 0 {
		text := "✅ 当前没有需要更新的容器"
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	b.sendUpdateNotificationToChatWithPage(chatID, updateContainers, page, messageID)
}
