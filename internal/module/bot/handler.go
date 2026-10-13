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
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/notify"
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
	if u.Message != nil && u.Message.Text != "" {
		b.handleCommand(chatID, u.Message.Text, u.Message.MessageID, shellMessageOwner(u.Message))
	}
}

// handleCommand 处理文本指令。msgID 为该文本消息的 ID（用于 Shell 会话删除命令消息）。
func (b *Bot) handleCommand(chatID int64, text string, msgID int64, userID int64) {
	trimmed := strings.TrimSpace(text)
	// 最高优先级：若处于 Shell 会话中，除退出指令外的所有文本都作为命令执行。
	if s := b.getShell(chatID); s != nil {
		if userID <= 0 || userID != s.ownerID {
			b.reply(chatID, "⛔ 此聊天的终端由其他用户占用，只有发起者可以输入或关闭。")
			return
		}
		switch trimmed {
		case "/exit", "/quit", "/cancel":
			b.finishShell(chatID, s)
		case "/menu", "/start":
			b.finishShell(chatID, s)
			b.sendMainMenu(chatID)
		default:
			b.runShellCommand(chatID, s, text, msgID)
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
		// 帮助：带返回主菜单键盘，与主菜单「📚 帮助」入口保持一致
		b.replyKeyboard(chatID, helpText(), &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
		}}})
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
	// 复用统一的二次确认逻辑（命令行按名操作默认本地主机）
	b.askConfirm(chatID, action, id, name, "")
}

// handleCallback 处理 inline 按钮回调：主菜单跳转、列表操作确认、执行已确认操作。
func (b *Bot) handleCallback(chatID int64, cb *telegram.CallbackQuery) {
	// 先解码并校验聊天绑定；失效按钮不能继续路由。
	decoded, err := b.decodeCallback(chatID, cb)
	if err != nil {
		_ = b.client.AnswerCallbackQuery(cb.ID, compactName(err.Error()))
		return
	}
	cb = decoded
	_ = b.client.AnswerCallbackQuery(cb.ID, "")
	var messageID int64 = 0
	if cb.Message != nil {
		messageID = cb.Message.MessageID
	}

	// 终端身份路由先于通用三段回调，旧按钮也必须严格绑定当前会话。
	if b.handleShellCallback(chatID, cb, messageID) {
		return
	}

	// 用户导航立即使旧后台刷新失效，避免确认页被检查结果覆盖。
	b.advanceUpdatePage(chatID, messageID)
	if cb.Data == "cancel" {
		if messageID > 0 {
			b.editMessage(chatID, messageID, "已取消。")
		} else {
			b.reply(chatID, "已取消。")
		}
		return
	}
	parts := strings.Split(cb.Data, "|")
	if parts[0] == "imgdetail" && len(parts) == 4 {
		page, _ := strconv.Atoi(parts[3])
		b.imageDetail(chatID, messageID, b.hostFromParts(parts, 1), parts[2], page)
		return
	}
	if parts[0] == "bkpg" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.backupListPage(chatID, messageID, page)
		return
	}
	if parts[0] == "bkdetail" && len(parts) >= 3 {
		page, _ := strconv.Atoi(parts[1])
		b.backupDetail(chatID, messageID, page, strings.Join(parts[2:], "|"))
		return
	}
	if parts[0] == "instpage" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.replyInstanceOverviewPage(chatID, messageID, page)
		return
	}
	if parts[0] == "instcontainers" && len(parts) == 3 {
		page, _ := strconv.Atoi(parts[2])
		b.replyInstanceContainers(chatID, messageID, b.hostFromParts(parts, 1), page)
		return
	}
	if parts[0] == "instdetail" && len(parts) == 2 {
		b.replyInstanceDetail(chatID, messageID, b.hostFromParts(parts, 1))
		return
	}
	if parts[0] == "us" {
		b.handleUpdateSession(chatID, messageID, parts)
		return
	}
	if parts[0] == "uphost" && len(parts) == 3 {
		page, _ := strconv.Atoi(parts[2])
		b.replyHostUpdates(chatID, messageID, b.hostFromParts(parts, 1), page)
		return
	}
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
			b.replyInstanceOverview(chatID, messageID)
		case "settings":
			// 设置中心：整合多个设置入口
			b.replySettingsCenter(chatID, messageID)
		case "compose":
			b.listComposeProjects(chatID, messageID, 0)
		case "mute":
			// 更新通知设置面板（page 来自 menu|mute|<page>，默认 0）
			b.sendMuteSettings(chatID, page, messageID)
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
	// 更新通知屏蔽切换（设置面板）：mute|<page>|<容器名>；容器名可能含 | 故用 Join 还原
	if parts[0] == "mute" && len(parts) >= 3 {
		page, _ := strconv.Atoi(parts[1])
		name := strings.Join(parts[2:], "|")
		b.toggleMute(chatID, name, page, messageID)
		return
	}
	// 单容器操作面板：panel|<id>|<name>[|<hostCode>]
	if parts[0] == "panel" && len(parts) >= 3 {
		host := b.hostFromParts(parts, 3)
		b.sendContainerPanel(chatID, parts[1], parts[2], host, messageID)
		return
	}
	// 面板子功能路由：<action>|<id>|<name>[|<hostCode>]
	if len(parts) == 3 || (len(parts) == 4 && isPanelSubAction(parts[0])) {
		id, name := parts[1], parts[2]
		host := b.hostFromParts(parts, 3)
		switch parts[0] {
		case "logs":
			b.sendContainerLogs(chatID, id, name, host, messageID)
			return
		case "logdl":
			// 下载完整日志：作为文档发送
			b.sendContainerLogFile(chatID, id, name, host)
			return
		case "inspect":
			b.sendContainerInspect(chatID, id, name, host, messageID)
			return
		case "stats":
			b.sendContainerStats(chatID, id, name, host, messageID)
			return
		case "tags":
			b.sendTagSwitch(chatID, id, name, host, messageID)
			return
		case "execp":
			// 在面板消息上直接开启 Shell（编辑本条消息），退出后恢复面板菜单
			b.promptExec(chatID, id, name, host, messageID, shellCallbackOwner(cb))
			return
		case "rename":
			b.promptRename(chatID, id, name, host)
			return
		case "tagin":
			b.promptTagInput(chatID, id, name, host)
			return
		}
	}
	// 切换 tag 执行：dotag|<id>|<name>|<tag>[|<hostCode>]
	if parts[0] == "dotag" && len(parts) >= 4 {
		host := b.hostFromParts(parts, 4)
		b.doSwitchTag(chatID, parts[1], parts[2], parts[3], host, messageID)
		return
	}
	// 列表操作按钮：act|<action>|<id>|<name>[|<hostCode>]
	if len(parts) >= 4 && parts[0] == "act" {
		action := parts[1]
		host := b.hostFromParts(parts, 4)
		// 低风险操作（启动/停止/重启/暂停/恢复/更新）直接执行；危险操作走二次确认
		switch action {
		case "update":
			b.confirmSingleUpdate(chatID, messageID, parts[2], host)
		case "start", "stop", "restart", "pause", "unpause":
			b.execAction(chatID, action, parts[2], parts[3], host, messageID)
		default:
			b.askConfirm(chatID, action, parts[2], parts[3], host)
		}
		return
	}
	// 二次确认通过：confirm|<action>|<id>|<name>[|<hostCode>]
	if len(parts) >= 4 && parts[0] == "confirm" {
		host := b.hostFromParts(parts, 4)
		if parts[1] == "update" {
			b.confirmSingleUpdate(chatID, messageID, parts[2], host)
			return
		}
		b.execAction(chatID, parts[1], parts[2], parts[3], host, messageID)
		return
	}
	// 批量更新确认：batchupdate|confirm
	if parts[0] == "batchupdate" && len(parts) == 2 && parts[1] == "confirm" {
		b.replyUpdateCenter(chatID, messageID, 0)
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
		// 重新扫描找到项目并执行（走 AppConfig 优先的配置）
		scanPaths, maxDepth := b.composeScanConfig()
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
	// 周期更新提醒「查看并管理」：updnotify|<page> —— 编辑摘要消息为分页详情（仅未屏蔽）
	if parts[0] == "updnotify" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.resendUpdateNotification(chatID, page, messageID, false)
		return
	}
	// 周期更新提醒「查看被屏蔽的」：updmuted|<page> —— 显示已屏蔽但有更新的容器
	if parts[0] == "updmuted" && len(parts) == 2 {
		page, _ := strconv.Atoi(parts[1])
		b.resendUpdateNotification(chatID, page, messageID, true)
		return
	}
	// 定时更新完成结果交互：rres|<action>|<ruleID>[|...]
	if parts[0] == "rres" && len(parts) >= 3 {
		ruleID := parts[2]
		switch parts[1] {
		case "sum": // 返回完成摘要
			if res := notify.GetRuleUpdateResultByExecutionID(ruleID); res != nil {
				b.sendRuleResultSummary(chatID, res, messageID)
			} else {
				b.editMessage(chatID, messageID, "⚠️ 该执行结果已过期（服务可能已重启）。")
			}
		case "skip", "fail": // 查看跳过/失败明细（parts[3]=页码）
			page := 0
			if len(parts) >= 4 {
				page = parsePageArg(parts[3])
			}
			b.sendRuleResultDetail(chatID, ruleID, parts[1], page, messageID)
		case "upd":
			if len(parts) >= 5 {
				b.editOrReplyKeyboard(chatID, messageID, "⚠️ 确认更新该执行记录中的容器？将沿用原有保留旧容器策略。", &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "✅ 确认", CallbackData: "rres|updrun|" + strings.Join(parts[2:], "|")}, {Text: "取消", CallbackData: "rres|sum|" + ruleID}}}})
			}
		case "updrun":
			if len(parts) >= 5 {
				b.updateResultItem(chatID, ruleID, parts[3], parsePageArg(parts[4]), messageID)
			}
		case "retry": // 重试全部失败（二次确认）
			b.confirmRetryFailed(chatID, ruleID, messageID)
		case "retrun": // 确认后执行批量重试
			b.executeRetryFailed(chatID, ruleID, messageID)
		}
		return
	}
	// 详情页就地切换通知屏蔽：mutehere|<page>|<容器名> —— 切换后不跳转，就地刷新当前页详情
	// 容器名可能含 |，放在末尾用 Join 还原
	if parts[0] == "mutehere" && len(parts) >= 3 {
		page, _ := strconv.Atoi(parts[1])
		name := strings.Join(parts[2:], "|")
		b.toggleMuteInPlace(chatID, name, page, messageID)
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
		// 操作前重新验证白名单和路径，陈旧按钮不允许访问其他文件。
		if parts[1] == "restore" || parts[1] == "restore_confirm" || parts[1] == "delete" || parts[1] == "delconf" {
			if len(parts) < 3 || !b.validBackupName(strings.Join(parts[2:], "|")) {
				b.editOrReplyKeyboard(chatID, messageID, "⚠️ 备份不存在或路径不安全", mainMenuKeyboard())
				return
			}
		}

		switch parts[1] {
		case "create":
			// 创建备份
			b.createBackup(chatID, messageID)
		case "list":
			// 查看备份列表
			b.listBackups(chatID, messageID)
		case "restore":
			if len(parts) >= 3 {
				name := strings.Join(parts[2:], "|")
				b.editOrReplyKeyboard(chatID, messageID, "⚠️ 确认恢复容器配置？\n这份备份不包含容器数据卷归档。\n<code>"+escapeHTML(name)+"</code>", &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "✅ 恢复配置", CallbackData: "backup|restore_confirm|" + name}, {Text: "取消", CallbackData: b.backupReturn(chatID, messageID)}}}})
			}
		case "restore_confirm":
			if len(parts) >= 3 {
				b.restoreBackup(chatID, strings.Join(parts[2:], "|"), messageID)
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
			// 清理本地未使用镜像（二次确认）
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
				b.resendUpdateNotification(chatID, page, messageID, false) // 默认显示未屏蔽
			}
		}
		return
	}
}

// execAction 统一执行容器操作并回报结果，供直接点击与二次确认后调用。
// messageID > 0 时用于编辑原消息（特别是更新操作的进度显示）。
func (b *Bot) execAction(chatID int64, action, id, name, hostID string, messageID int64) {
	// 按容器所属主机构建操作服务（多 Docker 管理），本地/远程统一
	ops := containerops.NewForHost(b.svcCtx, hostID)
	var err error
	switch action {
	case "start":
		err = ops.Start(id)
	case "stop":
		err = ops.Stop(id, 10)
	case "restart":
		err = ops.Restart(id, 10)
	case "pause":
		err = ops.Pause(id)
	case "unpause":
		err = ops.Unpause(id)
	case "kill":
		err = ops.Kill(id)
	case "remove":
		err = ops.Remove(id, true, false)
	case "update":
		b.confirmSingleUpdate(chatID, messageID, id, hostID)
		return
	default:
		b.reply(chatID, "不支持的操作")
		return
	}
	// 删除类操作后容器已不存在，返回列表；其余操作返回该容器管理面板
	var backKb *telegram.InlineKeyboardMarkup
	if action == "remove" {
		backKb = &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "⬅ 返回列表", CallbackData: b.returnMenu(chatID, messageID)},
		}}}
	} else {
		backKb = b.backToPanelKb(id, name, hostID)
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

// findContainer 按 id 前缀查找容器（本地，兼容旧调用）。
func (b *Bot) findContainer(id string) (MyType.Container, bool) {
	return b.findContainerOnHost(id, "")
}

// findContainerOnHost 在指定主机上按 id 前缀查找容器。hostID 为空则查本地。
func (b *Bot) findContainerOnHost(id, hostID string) (MyType.Container, bool) {
	list, err := utiles.GetContainerListFromHost(b.svcCtx, hostID)
	if err != nil {
		return MyType.Container{}, false
	}
	list = utiles.CheckImageUpdate(b.svcCtx, list)
	// 空身份或歧义前缀不能猜测目标；优先完整 ID 精确匹配。
	if id == "" {
		return MyType.Container{}, false
	}
	var found MyType.Container
	matches := 0
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
		if strings.HasPrefix(c.ID, id) {
			found = c
			matches++
		}
	}
	return found, matches == 1
}

// sendContainerPanel 推送单容器操作面板：把该容器所有可用操作收纳到二级菜单，
// 避免列表页按钮爆炸。按钮随容器状态动态变化。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendContainerPanel(chatID int64, id, name, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	id = c.ID
	name = containerName(c)
	running := strings.EqualFold(c.State, "running")
	paused := strings.EqualFold(c.State, "paused")
	// host 码后缀：拼到每个 callback 末尾，让子操作知道目标主机
	hc := encodeHostRef(hostID)
	hs := "|" + hc

	// 面板文字：状态 + 镜像 + 更新标识 + 来源主机
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>⚙ 容器管理：%s</b>\n", escapeHTML(name)))
	if hostID != "" && hostID != appconfig.DockerHostLocalID && c.HostName != "" {
		text.WriteString(fmt.Sprintf("来源：🖥 %s\n", escapeHTML(c.HostName)))
	}
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
			telegram.InlineKeyboardButton{Text: "⏹ 停止", CallbackData: fmt.Sprintf("act|stop|%s|%s%s", id, name, hs)},
			telegram.InlineKeyboardButton{Text: "🔄 重启", CallbackData: fmt.Sprintf("act|restart|%s|%s%s", id, name, hs)},
			telegram.InlineKeyboardButton{Text: "⏸ 暂停", CallbackData: fmt.Sprintf("act|pause|%s|%s%s", id, name, hs)},
		)
	} else if paused {
		lifeRow = append(lifeRow,
			telegram.InlineKeyboardButton{Text: "▶ 恢复", CallbackData: fmt.Sprintf("act|unpause|%s|%s%s", id, name, hs)},
			telegram.InlineKeyboardButton{Text: "⏹ 停止", CallbackData: fmt.Sprintf("act|stop|%s|%s%s", id, name, hs)},
		)
	} else {
		lifeRow = append(lifeRow,
			telegram.InlineKeyboardButton{Text: "▶ 启动", CallbackData: fmt.Sprintf("act|start|%s|%s%s", id, name, hs)},
		)
	}
	rows = append(rows, lifeRow)

	// 第二行：更新 / 切换 tag
	updRow := []telegram.InlineKeyboardButton{
		{Text: "⬆ 更新", CallbackData: fmt.Sprintf("act|update|%s|%s%s", id, name, hs)},
		{Text: "🏷 切换标签", CallbackData: fmt.Sprintf("tags|%s|%s%s", id, name, hs)},
	}
	rows = append(rows, updRow)

	// 第三行：信息查看 + 命令行
	infoRow := []telegram.InlineKeyboardButton{
		{Text: "📄 日志", CallbackData: fmt.Sprintf("logs|%s|%s%s", id, name, hs)},
		{Text: "🔍 详情", CallbackData: fmt.Sprintf("inspect|%s|%s%s", id, name, hs)},
		{Text: "📊 资源", CallbackData: fmt.Sprintf("stats|%s|%s%s", id, name, hs)},
		{Text: "💻 命令行", CallbackData: fmt.Sprintf("execp|%s|%s%s", id, name, hs)},
	}
	rows = append(rows, infoRow)

	// 第四行：重命名
	opRow := []telegram.InlineKeyboardButton{
		{Text: "✏ 重命名", CallbackData: fmt.Sprintf("rename|%s|%s%s", id, name, hs)},
	}
	rows = append(rows, opRow)

	// 第五行：危险操作（删除 / 强杀）
	dangerRow := []telegram.InlineKeyboardButton{
		{Text: "💀 强制终止", CallbackData: fmt.Sprintf("act|kill|%s|%s%s", id, name, hs)},
		{Text: "🗑 删除", CallbackData: fmt.Sprintf("act|remove|%s|%s%s", id, name, hs)},
	}
	rows = append(rows, dangerRow)

	// 第六行：返回列表
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回列表", CallbackData: b.returnMenu(chatID, messageID)},
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

// askConfirm 对指定容器操作弹出二次确认按钮。hostID 定位容器所属主机。
func (b *Bot) askConfirm(chatID int64, action, id, name, hostID string) {
	hs := "|" + encodeHostRef(hostID)
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "✅ 确认" + actionLabel(action), CallbackData: fmt.Sprintf("confirm|%s|%s|%s%s", action, id, name, hs)},
			{Text: "取消", CallbackData: "cancel"},
		}},
	}
	b.replyKeyboard(chatID, fmt.Sprintf("确认对容器 <b>%s</b> 执行「%s」？", escapeHTML(name), actionLabel(action)), kb)
}

// pageSize 每页展示的容器数量。
const pageSize = 8

// replyContainerList 分页推送容器列表，每个容器附带操作按钮。
// onlyRunning=true 时仅展示运行中容器；page 从 0 开始。
// messageID > 0 时编辑原消息（用于翻页），否则发送新消息。
func (b *Bot) replyContainerList(chatID int64, onlyRunning bool, page int, messageID int64) {
	mode := "ps"
	if onlyRunning {
		mode = "run"
	}
	b.renderContainerGrid(chatID, mode, page, messageID)
}

// imagesPageSize 镜像列表每页展示条数（避免单条消息过长）。
const imagesPageSize = 8

// replyImageList 分页推送镜像详细列表：每行显示 镜像名:标签 + 大小 + 是否在用。
// 按镜像大小倒序排列，大镜像在前方便用户识别存储占用大户。
// page 从 0 开始；messageID > 0 时编辑原消息（翻页），否则发送新消息。
func (b *Bot) replyImageList(chatID int64, messageID int64, page int) {
	b.renderImageList(chatID, messageID, page)
}

// replySystemOverview 推送系统资源概览：容器/镜像统计 + 磁盘占用汇总。
// 提供运行/停止容器数、镜像总数、使用中镜像数和总磁盘占用等关键指标。
// messageID > 0 时编辑原消息（用于从主菜单点击进入），否则发送新消息；
// 末尾统一附带「返回主菜单」按钮，保证菜单导航闭环。
func (b *Bot) replySystemOverview(chatID int64, messageID int64) {
	// 获取容器列表
	// 汇总所有已启用 Docker 主机的容器，使系统概览反映全部实例（而非仅本地）
	containers, errC := utiles.GetAllContainers(b.svcCtx)
	if errC != nil {
		b.reply(chatID, "获取容器信息失败："+errC.Error())
		return
	}

	// 镜像统计用「不去重」的逐主机列表：磁盘占用需按各主机各自累加才真实
	images, errI := utiles.GetAllImagesListPerHost(b.svcCtx)
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
func (b *Bot) checkAllUpdates(chatID int64) { b.recheckUpdates(chatID, 0) }

// updateAllContainers 批量更新所有有可用更新的容器（高风险操作，需二次确认）。
func (b *Bot) updateAllContainers(chatID int64) { b.replyUpdateCenter(chatID, 0, 0) }

// executeBatchUpdate 执行批量更新：依次为所有有更新的容器提交更新任务。
// messageID > 0 时把结果编辑到原消息（内联按钮触发），否则发新消息。
func (b *Bot) executeBatchUpdate(chatID int64, messageID int64) {
	b.replyUpdateCenter(chatID, messageID, 0)
}

// composePageSize Compose 项目列表每页展示条数（每项占一个按钮）。
const composePageSize = 8

// composeScanConfig 读取生效的 Compose 扫描配置：优先动态配置(AppConfig)，为空回退静态 yaml。
// 与 Web 端 ComposeLogic.scanPaths()/maxDepth() 保持一致，避免 Bot 只读静态 yaml 导致
// 用户在前端保存后 Bot 仍报“未配置扫描路径”。
func (b *Bot) composeScanConfig() (scanPaths []string, maxDepth int) {
	dyn := b.svcCtx.AppConfig.Get().Compose
	if len(dyn.ScanPaths) > 0 {
		scanPaths = dyn.ScanPaths
	} else {
		scanPaths = b.svcCtx.Config.Compose.ScanPaths
	}
	if dyn.MaxDepth > 0 {
		maxDepth = dyn.MaxDepth
	} else {
		maxDepth = b.svcCtx.Config.Compose.MaxDepth
	}
	return scanPaths, maxDepth
}

// listComposeProjects 分页列出扫描到的 Compose 项目，每个项目一个按钮进入管理面板。
// page 从 0 开始；messageID > 0 时编辑原消息（翻页），否则发送新消息。
func (b *Bot) listComposeProjects(chatID int64, messageID int64, page int) {
	// 从配置获取扫描路径和深度（走 AppConfig 优先的配置）
	scanPaths, maxDepth := b.composeScanConfig()
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
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n\n", i+1, escapeHTML(proj.Name), escapeHTML(compactName(proj.Dir))))
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
	// 重新扫描找到该项目（走 AppConfig 优先的配置）
	scanPaths, maxDepth := b.composeScanConfig()
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
	// 重新扫描找到项目（走 AppConfig 优先的配置）
	scanPaths, maxDepth := b.composeScanConfig()
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
	// 编辑与首次发送都使用服务端短令牌，完整容器身份不会被截断。
	encoded, err := b.encodeKeyboard(chatID, keyboard)
	if err != nil {
		logx.Errorf("编码键盘失败: %v", err)
		return
	}
	keyboard = encoded
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
const updateCenterPageSize = 8

// replyUpdateCenter 更新中心：分页显示所有有可用更新的容器列表，提供一键检查和批量更新。
// page 从 0 开始（仅对待更新列表分页）；messageID > 0 时编辑原消息。
func (b *Bot) replyUpdateCenter(chatID int64, messageID int64, page int) {
	b.replyHostUpdates(chatID, messageID, "*", page)
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
			{{Text: "🔕 更新通知设置", CallbackData: "menu|mute"}, {Text: "⏱ 检查间隔", CallbackData: "notify|interval"}},
			{{Text: "🔕 全部静默", CallbackData: "notify|muteall"}},
			{{Text: "🧩 Compose 项目", CallbackData: "menu|compose"}},
			{{Text: "🗑️ 清理本地未使用镜像", CallbackData: "settings|prune"}},
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
	b.renderContainerGrid(chatID, "stopped", page, messageID)
}

// recheckUpdates 重新检查所有容器的镜像更新并刷新更新中心。
func (b *Bot) recheckUpdates(chatID int64, messageID int64) {
	// 先展示已有缓存，后台检查完成后按消息版本原地刷新。
	b.replyHostUpdates(chatID, messageID, "*", 0)
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
func (b *Bot) listBackups(chatID int64, messageID int64) { b.backupReturnPage(chatID, messageID) }

// restoreBackup 从备份恢复容器（提交后台任务）。
// messageID > 0 时把进度/结果编辑到原消息，保持单条消息流。
func (b *Bot) restoreBackup(chatID int64, filename string, messageID int64) {
	backKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回备份列表", CallbackData: b.backupReturn(chatID, messageID)},
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
				{Text: "❌ 取消", CallbackData: b.backupReturn(chatID, messageID)},
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
	text.WriteString("<b>🗑️ 清理本地未使用镜像</b>\n\n")
	text.WriteString("请选择清理范围：\n\n")
	text.WriteString("<b>1. 悬空镜像（Dangling）</b>\n")
	text.WriteString("仅清理无标签的镜像（&lt;none&gt;），较为安全。\n\n")
	text.WriteString("<b>2. 未使用镜像（Unused）</b>\n")
	text.WriteString("清理所有未被任何容器使用的镜像，包括有标签但未使用的镜像。\n")
	text.WriteString("⚠️ 此操作可能会删除暂时不用但以后需要的镜像。")

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🗑️ 清理悬空镜像", CallbackData: "settings|prune_dangling"}},
			{{Text: "⚠️ 清理本地未使用镜像", CallbackData: "settings|prune_unused"}},
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

// sendUpdateNotificationToChat 周期检测的更新推送入口：只发精简摘要 + 一个「查看并管理」按钮，
// 避免一次性铺开大量容器按钮刷屏。点按钮后（回调 updnotify|0）再进入分页详情。
// 仅在有可更新容器时才会被调用（调用方 notifier 已保证 len>0 且已排除屏蔽容器）。
// muted 为已屏蔽但有更新的容器列表，非空时额外提供"查看被屏蔽的"按钮。
func (b *Bot) sendUpdateNotificationToChat(chatID int64, containers []UpdateContainer, muted []UpdateContainer) {
	if len(containers) == 0 && len(muted) == 0 {
		return
	}
	// 按稳定实例 ID 分组，按钮只跳转该实例，绝不借用排序下标。
	groups := make(map[string][]UpdateContainer)
	for _, c := range containers {
		groups[c.HostID] = append(groups[c.HostID], c)
	}
	hosts := make([]string, 0, len(groups))
	for host := range groups {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>🔔 可用更新</b> · %d 项\n", len(containers)))
	var rows [][]telegram.InlineKeyboardButton
	for i, host := range hosts {
		if i >= 12 {
			text.WriteString(fmt.Sprintf("… 另 %d 个实例，进入更新中心查看\n", len(hosts)-i))
			break
		}
		items := groups[host]
		name := items[0].HostName
		if name == "" {
			name = host
		}
		if name == "" || name == appconfig.DockerHostLocalID {
			name = "本地"
		}
		text.WriteString(fmt.Sprintf("• %s：%d 项\n", escapeHTML(compactName(name)), len(items)))
		addGridButton(&rows, telegram.InlineKeyboardButton{Text: fmt.Sprintf("🖥 %s · %d", compactName(name), len(items)), CallbackData: "us|" + b.notificationSnapshot(chatID, items) + "|page|0"}, i)
	}
	if len(muted) > 0 {
		text.WriteString(fmt.Sprintf("💤 已静默 %d 项\n", len(muted)))
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "查看静默项", CallbackData: "updmuted|0"}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "更新中心", CallbackData: "menu|updates"}, {Text: "通知设置", CallbackData: "menu|mute"}})
	b.replyKeyboard(chatID, text.String(), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

// sendUpdateNotificationToChatWithPage 向指定会话发送带分页的更新通知（详情页，含逐容器操作按钮）。
func (b *Bot) sendUpdateNotificationToChatWithPage(chatID int64, containers []UpdateContainer, page int, messageID int64) {
	// 通知详情也使用不可变完整身份快照，沿用通知设置入口而不截短操作名称。
	items := make([]MyType.Container, 0, len(containers))
	for _, c := range containers {
		item := MyType.Container{CreateImage: c.Image, HostID: c.HostID, HostName: c.HostName, Update: true}
		item.ID = c.ID
		item.Names = []string{c.Name}
		item.Image = c.Image
		items = append(items, item)
	}
	token := b.saveUpdateSnapshot(chatID, items, "menu|updates")
	b.renderUpdateSnapshot(chatID, messageID, token, page)
}

// confirmMuteAll 确认全部屏蔽更新通知（二次确认）。
func (b *Bot) confirmMuteAll(chatID int64, messageID int64) {
	// 获取所有有更新的容器
	// 汇总所有已启用 Docker 主机的容器（屏蔽列表覆盖远程主机）
	containers, err := utiles.GetAllContainers(b.svcCtx)
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
	// 汇总所有已启用 Docker 主机的容器（执行全部屏蔽覆盖远程主机）
	containers, err := utiles.GetAllContainers(b.svcCtx)
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
// mutedOnly: true 时只显示已屏蔽的容器，false 时只显示未屏蔽的容器。
func (b *Bot) resendUpdateNotification(chatID int64, page int, messageID int64, mutedOnly bool) {
	// 获取所有主机的有更新容器：与周期检测摘要（scheduler 的 pending）保持同一数据源，
	// 避免"摘要用 GetAllContainers（含远程主机）、详情用 GetContainerList（仅本地）"导致两者内容对不上。
	containers, err := utiles.GetAllContainers(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}
	containers = utiles.CheckImageUpdate(b.svcCtx, containers)

	// 获取屏蔽列表
	cfg := b.svcCtx.AppConfig.Get()
	mutedSet := make(map[string]struct{}, len(cfg.Telegram.MutedContainers))
	for _, m := range cfg.Telegram.MutedContainers {
		mutedSet[m] = struct{}{}
	}

	// 筛选有更新的容器，按 mutedOnly 参数过滤
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

		// 根据 mutedOnly 决定是否包含此容器
		_, isMuted := mutedSet[name]
		if mutedOnly && !isMuted {
			continue // 只显示屏蔽的，跳过未屏蔽的
		}
		if !mutedOnly && isMuted {
			continue // 只显示未屏蔽的，跳过屏蔽的
		}

		// 优先使用 CreateImage，避免镜像更新后 Image 字段变空或变成 SHA256
		imageToUse := c.CreateImage
		if imageToUse == "" {
			imageToUse = c.Image // 降级使用 Image 字段
		}
		updateContainers = append(updateContainers, UpdateContainer{
			ID: c.ID, Name: name, Image: imageToUse, HostID: c.HostID, HostName: c.HostName,
		})
	}

	if len(updateContainers) == 0 {
		var text string
		if mutedOnly {
			text = "✅ 当前没有已屏蔽的容器需要更新"
		} else {
			text = "✅ 当前没有未屏蔽的容器需要更新"
		}
		if messageID > 0 {
			b.editMessage(chatID, messageID, text)
		} else {
			b.reply(chatID, text)
		}
		return
	}

	b.sendUpdateNotificationToChatWithPage(chatID, updateContainers, page, messageID)
}
