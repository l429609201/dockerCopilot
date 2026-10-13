package bot

import (
	"fmt"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// 更新会话的目标一经展示便不可变确认只消费一次，不重新获取动态列表或按名称找替代容器。
type updateSnapshot struct {
	bot      *Bot
	chat     int64
	expires  time.Time
	items    []MyType.Container
	back     string
	used     bool
	confirm  bool
	children map[string]string
	direct   bool
}

var updateSnapshots = struct {
	sync.Mutex
	entries map[string]updateSnapshot
}{entries: make(map[string]updateSnapshot)}

func (b *Bot) saveUpdateSnapshot(chat int64, items []MyType.Container, back string) string {
	updateSnapshots.Lock()
	defer updateSnapshots.Unlock()
	for k, s := range updateSnapshots.entries {
		if time.Now().After(s.expires) {
			delete(updateSnapshots.entries, k)
		}
	}
	// 硬上限避免大量按钮会话造成无限增长，过量时淘汰最早过期条目。
	if len(updateSnapshots.entries) >= 512 {
		var oldest string
		var expiry time.Time
		for k, v := range updateSnapshots.entries {
			if oldest == "" || v.expires.Before(expiry) {
				oldest = k
				expiry = v.expires
			}
		}
		delete(updateSnapshots.entries, oldest)
	}
	token := uuid.NewString()
	// 复制名称切片，外部刷新不得改变已确认快照的展示身份。
	items = append([]MyType.Container(nil), items...)
	for i := range items {
		items[i].Names = append([]string(nil), items[i].Names...)
	}
	updateSnapshots.entries[token] = updateSnapshot{bot: b, chat: chat, expires: time.Now().Add(30 * time.Minute), items: append([]MyType.Container(nil), items...), back: back}
	return token
}
func (b *Bot) getUpdateSnapshot(chat int64, token string, consume bool) (updateSnapshot, bool) {
	updateSnapshots.Lock()
	defer updateSnapshots.Unlock()
	s, ok := updateSnapshots.entries[token]
	if !ok || s.bot != b || s.chat != chat || s.used || time.Now().After(s.expires) {
		return updateSnapshot{}, false
	}
	if consume && !s.confirm {
		return updateSnapshot{}, false
	}
	if consume {
		s.used = true
		updateSnapshots.entries[token] = s
	}
	// 返回独立副本，执行排序与并发读者不能改变存储中的快照。
	s.items = append([]MyType.Container(nil), s.items...)
	for i := range s.items {
		s.items[i].Names = append([]string(nil), s.items[i].Names...)
	}
	children := make(map[string]string, len(s.children))
	for key, value := range s.children {
		children[key] = value
	}
	s.children = children
	return s, true
}
func updateImage(c MyType.Container) string {
	if c.CreateImage != "" {
		return c.CreateImage
	}
	return c.Image
}
func (b *Bot) replyHostUpdates(chat, message int64, host string, page int) {
	if message == 0 {
		var err error
		message, err = b.client.SendMessageReturnID(chat, "⏳ 正在读取更新缓存…", nil)
		if err != nil {
			return
		}
	}
	var list []MyType.Container
	var err error
	if host == "*" {
		list, err = utiles.GetAllContainers(b.svcCtx)
	} else {
		list, err = utiles.GetContainerListFromHost(b.svcCtx, host)
	}
	if err != nil {
		b.editOrReplyKeyboard(chat, message, "❌ 获取更新失败："+escapeHTML(err.Error()), mainMenuKeyboard())
		return
	}
	list = utiles.CheckImageUpdate(b.svcCtx, list)
	items := make([]MyType.Container, 0, len(list))
	for _, c := range list {
		if c.Update {
			items = append(items, c)
		}
	}
	token := b.saveUpdateSnapshot(chat, items, "menu|updates")
	b.renderUpdateSnapshot(chat, message, token, page)
	b.checkUpdatesInBackground(chat, message, host, page)
}
func (b *Bot) renderUpdateSnapshot(chat, message int64, token string, page int) {
	s, ok := b.getUpdateSnapshot(chat, token, false)
	if !ok {
		b.expiredUpdate(chat, message)
		return
	}
	page, pages, start, end := pageBounds(len(s.items), page, updateCenterPageSize)
	var rows [][]telegram.InlineKeyboardButton
	for i, c := range s.items[start:end] {
		addGridButton(&rows, telegram.InlineKeyboardButton{Text: containerButton(c), CallbackData: fmt.Sprintf("us|%s|one|%d|%d", token, start+i, page)}, i)
	}
	var muteRows [][]telegram.InlineKeyboardButton
	for i, c := range s.items[start:end] {
		muted := false
		for _, name := range b.svcCtx.AppConfig.Get().Telegram.MutedContainers {
			if name == containerName(c) {
				muted = true
				break
			}
		}
		icon := "🔔"
		if muted {
			icon = "🔕"
		}
		addGridButton(&muteRows, telegram.InlineKeyboardButton{Text: icon + " " + compactName(containerName(c)), CallbackData: fmt.Sprintf("us|%s|mute|%d|%d", token, start+i, page)}, i)
	}
	rows = append(rows, muteRows...)
	rows = append(rows, buildPager("us|"+token+"|page", page, pages, "menu|home")...)
	if len(s.items) > 0 {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: fmt.Sprintf("⬆ 更新快照全部 %d 项", len(s.items)), CallbackData: fmt.Sprintf("us|%s|all|%d", token, page)}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "🔍 检查", CallbackData: "updc|check"}, {Text: "🔕 通知设置", CallbackData: "menu|mute"}})
	text := fmt.Sprintf("<b>⬆ 更新中心</b> · %d 项 · %d/%d\n选择单项或批量更新，执行前确认。", len(s.items), page+1, pages)
	if len(s.items) == 0 {
		text = "<b>⬆ 更新中心</b>\n当前未检测到可用更新。"
	}
	b.editOrReplyKeyboard(chat, message, text, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}
func (b *Bot) expiredUpdate(chat, message int64) {
	b.editOrReplyKeyboard(chat, message, "⚠️ 更新确认已失效或已执行，请重新选择。", &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "返回更新中心", CallbackData: "menu|updates"}}}})
}
func (b *Bot) handleUpdateSession(chat, message int64, parts []string) {
	if len(parts) < 3 {
		return
	}
	token, action := parts[1], parts[2]
	if action == "cancel" {
		s, ok := b.getUpdateSnapshot(chat, token, true)
		if !ok {
			b.expiredUpdate(chat, message)
			return
		}
		b.editOrReplyKeyboard(chat, message, "已取消更新。", &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "⬅ 返回原页", CallbackData: s.back}}}})
		return
	}
	if action == "run" {
		s, ok := b.getUpdateSnapshot(chat, token, true)
		if !ok {
			b.expiredUpdate(chat, message)
			return
		}
		b.runUpdateSnapshot(chat, message, s)
		return
	}
	s, ok := b.getUpdateSnapshot(chat, token, false)
	if !ok {
		b.expiredUpdate(chat, message)
		return
	}
	if action == "mute" && len(parts) > 4 {
		index, e := strconv.Atoi(parts[3])
		page, _ := strconv.Atoi(parts[4])
		if e != nil || index < 0 || index >= len(s.items) {
			return
		}
		b.setMuteState(containerName(s.items[index]))
		b.renderUpdateSnapshot(chat, message, token, page)
		return
	}
	page := 0
	if len(parts) > 3 {
		page, _ = strconv.Atoi(parts[3])
	}
	if action == "page" {
		b.renderUpdateSnapshot(chat, message, token, page)
		return
	}
	items := s.items
	if action == "one" {
		index := page
		if index < 0 || index >= len(items) {
			b.expiredUpdate(chat, message)
			return
		}
		items = items[index : index+1]
		page = 0
		if len(parts) > 4 {
			page, _ = strconv.Atoi(parts[4])
		}
	} else if action != "all" {
		return
	}
	if len(items) == 0 {
		b.expiredUpdate(chat, message)
		return
	}
	back := fmt.Sprintf("us|%s|page|%d", token, page)
	if s.direct {
		back = s.back
	}
	// 相同源快照与选择复用确认，重复点击不会生成多个可执行授权。
	choice := strings.Join(parts[2:], "|")
	updateSnapshots.Lock()
	parent := updateSnapshots.entries[token]
	confirm := parent.children[choice]
	updateSnapshots.Unlock()
	if confirm == "" {
		confirm = b.saveUpdateSnapshot(chat, items, back)
		updateSnapshots.Lock()
		parent = updateSnapshots.entries[token]
		if parent.children == nil {
			parent.children = make(map[string]string)
		}
		if existing := parent.children[choice]; existing != "" {
			delete(updateSnapshots.entries, confirm)
			confirm = existing
		} else {
			parent.children[choice] = confirm
			updateSnapshots.entries[token] = parent
			child := updateSnapshots.entries[confirm]
			child.confirm = true
			updateSnapshots.entries[confirm] = child
		}
		updateSnapshots.Unlock()
	}
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>⚠️ 确认更新 %d 项</b>\n将拉取镜像并重建，服务会短暂中断。\n", len(items)))
	for i, c := range items {
		if i >= 8 {
			text.WriteString(fmt.Sprintf("… 另 %d 项（同一快照）\n", len(items)-i))
			break
		}
		text.WriteString(fmt.Sprintf("• %s · %s\n", escapeHTML(compactName(containerName(c))), escapeHTML(compactName(c.HostName))))
	}
	b.editOrReplyKeyboard(chat, message, text.String(), &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "✅ 确认更新", CallbackData: "us|" + confirm + "|run"}, {Text: "取消", CallbackData: "us|" + confirm + "|cancel"}}}})
}
func (b *Bot) confirmSingleUpdate(chat, message int64, id, host string) {
	c, ok := b.findContainerOnHost(id, host)
	if !ok {
		b.editOrReplyKeyboard(chat, message, "❌ 容器不存在或实例不可用", mainMenuKeyboard())
		return
	}
	token := b.saveUpdateSnapshot(chat, []MyType.Container{c}, b.returnMenu(chat, message))
	updateSnapshots.Lock()
	single := updateSnapshots.entries[token]
	single.direct = true
	updateSnapshots.entries[token] = single
	updateSnapshots.Unlock()
	b.handleUpdateSession(chat, message, []string{"us", token, "one", "0", "0"})
}

// runUpdateSnapshot 顺序提交并等待真实终态，提交成功、无进度、超时都不冒充更新成功。
func (b *Bot) runUpdateSnapshot(chat, message int64, s updateSnapshot) {
	if message == 0 {
		var err error
		message, err = b.client.SendMessageReturnID(chat, "⏳ 正在提交更新…", nil)
		if err != nil {
			return
		}
	}
	b.editMessageKeyboard(chat, message, fmt.Sprintf("⏳ 正在更新 %d 项…", len(s.items)), nil)
	go func() {
		// 自更新放最后，辅助容器交接仅标记待确认，不冒充本进程见证完成。
		isSelf := func(c MyType.Container) bool {
			return (c.HostID == "" || c.HostID == appconfig.DockerHostLocalID) && utiles.IsSelfContainer(b.svcCtx, c.ID)
		}
		sort.SliceStable(s.items, func(i, j int) bool { return !isSelf(s.items[i]) && isSelf(s.items[j]) })
		success, failed, unknown := 0, 0, 0
		var details []string
		for i, c := range s.items {
			prefix := fmt.Sprintf("<b>⬆ 更新进度</b> · %d/%d\n✅ %d　❌ %d　⚠️ %d\n%s", i+1, len(s.items), success, failed, unknown, escapeHTML(compactName(containerName(c))))
			task, err := containerops.NewForHost(b.svcCtx, c.HostID).UpdateWithOptions(c.ID, updateImage(c), containerops.UpdateOptions{})
			if err != nil {
				failed++
				details = append(details, "❌ "+containerName(c)+"："+err.Error())
				continue
			}
			if isSelf(c) {
				unknown++
				details = append(details, "⚠️ 自更新已提交，重启后核验结果")
				break
			}
			timeout := b.svcCtx.Config.Task.PullTimeoutSec
			if timeout <= 0 {
				timeout = 1800
			}
			deadline := time.Now().Add(time.Duration(timeout+60) * time.Second)
			last := ""
			done := false
			for time.Now().Before(deadline) {
				p, exists := b.svcCtx.GetProgress(task)
				text := prefix + "\n⏳ 等待任务进度"
				if exists {
					text = prefix + fmt.Sprintf("\n%d%% · %s", p.Percentage, escapeHTML(compactName(p.Message)))
					if p.IsDone {
						done = true
						if p.Failed || p.Canceled {
							failed++
							details = append(details, "❌ "+containerName(c)+"："+p.Message)
						} else {
							success++
						}
						break
					}
				}
				if text != last {
					b.editMessage(chat, message, text)
					last = text
				}
				time.Sleep(2 * time.Second)
			}
			if !done {
				unknown++
				details = append(details, "⚠️ "+containerName(c)+"：等待超时，任务可能仍在执行")
				break
			}
		}
		text := fmt.Sprintf("<b>⬆ 更新结果</b>\n✅ 完成 %d　❌ 失败 %d　⚠️ 未确认 %d", success, failed, unknown)
		if pending := len(s.items) - success - failed - unknown; pending > 0 {
			text += fmt.Sprintf("\n⏸ 未提交 %d（上项超时，停止后续提交）", pending)
		}
		for i, line := range details {
			if i >= 8 {
				break
			}
			text += "\n" + escapeHTML(compactName(line))
		}
		b.editMessageKeyboard(chat, message, text, &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "⬅ 返回原页", CallbackData: s.back}, {Text: "刷新更新", CallbackData: "menu|updates"}}}})
	}()
}

// notificationSnapshot 固定通知实际展示范围，静默容器不会混进全部更新。
func (b *Bot) notificationSnapshot(chat int64, containers []UpdateContainer) string {
	items := make([]MyType.Container, 0, len(containers))
	for _, c := range containers {
		item := MyType.Container{HostID: c.HostID, HostName: c.HostName, CreateImage: c.Image, Update: true}
		item.ID = c.ID
		item.Names = []string{c.Name}
		item.Image = c.Image
		items = append(items, item)
	}
	return b.saveUpdateSnapshot(chat, items, "menu|updates")
}
