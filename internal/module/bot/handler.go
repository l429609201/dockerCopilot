package bot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
		b.handleCommand(chatID, strings.TrimSpace(u.Message.Text))
	}
}

// handleCommand 处理文本指令。
func (b *Bot) handleCommand(chatID int64, text string) {
	// 优先处理等待输入的会话动作（重命名/命令行）
	if p := b.takePending(chatID); p != nil {
		// /cancel 可取消待输入
		if strings.TrimSpace(text) == "/cancel" {
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
		b.replyImageList(chatID, 0)
	case "/sys":
		b.replySystemOverview(chatID)
	case "/check_updates":
		b.checkAllUpdates(chatID)
	case "/update_all":
		b.updateAllContainers(chatID)
	case "/compose":
		b.listComposeProjects(chatID, 0)
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
		case "images":
			b.replyImageList(chatID, messageID)
		case "sys":
			b.replySystemOverview(chatID)
		case "compose":
			b.listComposeProjects(chatID, messageID)
		case "help":
			b.reply(chatID, helpText())
		}
		return
	}
	// 单容器操作面板：panel|<id>|<name>
	if parts[0] == "panel" && len(parts) == 3 {
		b.sendContainerPanel(chatID, parts[1], parts[2])
		return
	}
	// 面板子功能路由
	if len(parts) == 3 {
		id, name := parts[1], parts[2]
		switch parts[0] {
		case "logs":
			b.sendContainerLogs(chatID, id, name)
			return
		case "inspect":
			b.sendContainerInspect(chatID, id, name)
			return
		case "stats":
			b.sendContainerStats(chatID, id, name)
			return
		case "tags":
			b.sendTagSwitch(chatID, id, name)
			return
		case "execp":
			b.promptExec(chatID, id, name)
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
		b.doSwitchTag(chatID, parts[1], parts[2], parts[3])
		return
	}
	// 列表操作按钮：act|<action>|<id>|<name>
	if len(parts) == 4 && parts[0] == "act" {
		action := parts[1]
		// 低风险操作（启动/停止/重启/暂停/恢复/更新）直接执行；危险操作走二次确认
		switch action {
		case "start", "stop", "restart", "pause", "unpause", "update":
			b.execAction(chatID, action, parts[2], parts[3])
		default:
			b.askConfirm(chatID, action, parts[2], parts[3])
		}
		return
	}
	// 二次确认通过：confirm|<action>|<id>|<name>
	if len(parts) == 4 && parts[0] == "confirm" {
		b.execAction(chatID, parts[1], parts[2], parts[3])
		return
	}
	// 批量更新确认：batchupdate|confirm
	if parts[0] == "batchupdate" && len(parts) == 2 && parts[1] == "confirm" {
		b.executeBatchUpdate(chatID)
		return
	}
	// Compose 项目列表：cmpls
	if parts[0] == "cmpls" {
		b.listComposeProjects(chatID, messageID)
		return
	}
	// Compose 项目面板：cmpp|<projectID>
	if parts[0] == "cmpp" && len(parts) == 2 {
		b.showComposeProjectPanel(chatID, parts[1])
		return
	}
	// Compose 执行动作：cmpa|<projectID>|<action>
	if parts[0] == "cmpa" && len(parts) == 3 {
		b.executeComposeAction(chatID, parts[1], parts[2])
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
			b.reply(chatID, "❌ 项目不存在或已被移除")
			return
		}
		b.doComposeAction(chatID, target, parts[2])
		return
	}
}

// execAction 统一执行容器操作并回报结果，供直接点击与二次确认后调用。
func (b *Bot) execAction(chatID int64, action, id, name string) {
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
		b.doUpdate(chatID, id, name)
		return
	default:
		b.reply(chatID, "不支持的操作")
		return
	}
	if err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 容器 %s 执行「%s」失败：%s", name, actionLabel(action), err.Error()))
		return
	}
	b.reply(chatID, fmt.Sprintf("✅ 容器 %s 已%s", name, actionLabel(action)))
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
func (b *Bot) sendContainerPanel(chatID int64, id, name string) {
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

	// 第三行：信息查看
	infoRow := []telegram.InlineKeyboardButton{
		{Text: "📄 日志", CallbackData: fmt.Sprintf("logs|%s|%s", id, name)},
		{Text: "🔍 详情", CallbackData: fmt.Sprintf("inspect|%s|%s", id, name)},
		{Text: "📊 资源", CallbackData: fmt.Sprintf("stats|%s|%s", id, name)},
	}
	rows = append(rows, infoRow)

	// 第四行：命令行 / 重命名
	opRow := []telegram.InlineKeyboardButton{
		{Text: "💻 命令行", CallbackData: fmt.Sprintf("execp|%s|%s", id, name)},
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
	b.replyKeyboard(chatID, text.String(), kb)
}

// sendMainMenu 推送主菜单（按钮式交互入口）。
func (b *Bot) sendMainMenu(chatID int64) {
	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "▶ 运行中容器", CallbackData: "menu|run|0"}},
			{{Text: "📦 全部容器", CallbackData: "menu|ps|0"}},
			{{Text: "🖼 镜像信息", CallbackData: "menu|images"}, {Text: "💻 系统概览", CallbackData: "menu|sys"}},
			{{Text: "🧩 Compose 项目", CallbackData: "menu|compose"}},
			{{Text: "❓ 帮助", CallbackData: "menu|help"}},
		},
	}
	b.replyKeyboard(chatID, "<b>DockerCopilot 控制台</b>\n请选择操作：", kb)
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

		// 每行按钮：常用操作 + 「管理」进入面板
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
		if c.Update {
			row = append(row, telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.⬆更新", seq), CallbackData: fmt.Sprintf("act|update|%s|%s", id, name)})
		}
		row = append(row, telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d.⚙管理", seq), CallbackData: fmt.Sprintf("panel|%s|%s", id, name)})
		rows = append(rows, row)
	}

	// 最后一行：上一页 / 下一页
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

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息（翻页场景）；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// replyImageList 推送镜像详细列表：每行显示 镜像名:标签 + 大小 + 是否在用。
// 按镜像大小倒序排列，大镜像在前方便用户识别存储占用大户。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) replyImageList(chatID int64, messageID int64) {
	list, err := utiles.GetImagesList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "获取镜像列表失败："+err.Error())
		return
	}
	if len(list) == 0 {
		b.reply(chatID, "📦 当前没有任何镜像")
		return
	}

	// 按大小倒序排列（大镜像优先显示）
	sort.Slice(list, func(i, j int) bool {
		return list[i].Size > list[j].Size
	})

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>📦 镜像列表（共 %d 个）</b>\n\n", len(list)))

	for _, img := range list {
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

	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessage(chatID, messageID, text.String())
	} else {
		b.reply(chatID, text.String())
	}
}

// replySystemOverview 推送系统资源概览：容器/镜像统计 + 磁盘占用汇总。
// 提供运行/停止容器数、镜像总数、使用中镜像数和总磁盘占用等关键指标。
func (b *Bot) replySystemOverview(chatID int64) {
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

	b.reply(chatID, text.String())
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

	b.reply(chatID, text.String())
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
func (b *Bot) executeBatchUpdate(chatID int64) {
	// 获取所有需要更新的容器
	containers, err := utiles.GetContainerList(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
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
		b.reply(chatID, "✅ 没有容器需要更新")
		return
	}

	b.reply(chatID, fmt.Sprintf("🚀 开始批量更新 %d 个容器...\n", len(needUpdateList)))

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

	b.reply(chatID, text.String())
}

// listComposeProjects 列出所有扫描到的 Compose 项目，每个项目一个按钮进入管理面板。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) listComposeProjects(chatID int64, messageID int64) {
	// 从配置获取扫描路径和深度
	scanPaths := b.svcCtx.Config.Compose.ScanPaths
	maxDepth := b.svcCtx.Config.Compose.MaxDepth
	if len(scanPaths) == 0 {
		b.reply(chatID, "❌ 未配置 Compose 项目扫描路径")
		return
	}

	scanner := compose.NewScanner(scanPaths, maxDepth)
	projects := scanner.Scan()

	if len(projects) == 0 {
		b.reply(chatID, "📂 未找到任何 Compose 项目\n\n请检查扫描路径配置。")
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>🧩 Compose 项目列表（共 %d 个）</b>\n\n", len(projects)))

	var rows [][]telegram.InlineKeyboardButton
	for i, proj := range projects {
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   <code>%s</code>\n\n", i+1, escapeHTML(proj.Name), escapeHTML(proj.Dir)))
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d. ⚙ 管理 %s", i+1, proj.Name), CallbackData: fmt.Sprintf("cmpp|%s", proj.ID)},
		})
	}

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}

	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text.String(), kb)
	} else {
		b.replyKeyboard(chatID, text.String(), kb)
	}
}

// showComposeProjectPanel 展示单个 Compose 项目的操作面板，提供 up/down/restart/pull/stop/start 按钮。
func (b *Bot) showComposeProjectPanel(chatID int64, projectID string) {
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
		b.reply(chatID, "❌ 项目不存在或已被移除")
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
	b.replyKeyboard(chatID, text.String(), kb)
}

// executeComposeAction 执行 Compose 动作（危险操作如 down 需二次确认）。
func (b *Bot) executeComposeAction(chatID int64, projectID, action string) {
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
		b.reply(chatID, "❌ 项目不存在或已被移除")
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
		b.replyKeyboard(chatID, text, kb)
		return
	}

	// 非危险操作直接执行
	b.doComposeAction(chatID, target, action)
}

// doComposeAction 实际执行 Compose 动作并回报结果。
func (b *Bot) doComposeAction(chatID int64, proj *compose.Project, action string) {
	if !compose.IsSupportedAction(action) {
		b.reply(chatID, "❌ 不支持的操作："+action)
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

	b.reply(chatID, fmt.Sprintf("🚀 正在执行 <b>%s</b> - %s...", escapeHTML(proj.Name), actionLabel))

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
	b.replyKeyboard(chatID, text.String(), kb)
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
