package bot

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	MyType "github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// 每条消息独立保存来源页，容器操作子页返回时不受其他聊天或消息影响。
type menuLocationKey struct {
	bot           *Bot
	chat, message int64
}
type menuLocation struct {
	callback string
	expires  time.Time
}

var menuLocations = struct {
	sync.Mutex
	entries map[menuLocationKey]menuLocation
}{entries: make(map[menuLocationKey]menuLocation)}

func (b *Bot) rememberMenu(chat, message int64, callback string) {
	if message == 0 {
		return
	}
	menuLocations.Lock()
	defer menuLocations.Unlock()
	for key, value := range menuLocations.entries {
		if time.Now().After(value.expires) {
			delete(menuLocations.entries, key)
		}
	}
	if len(menuLocations.entries) >= 2048 {
		for key := range menuLocations.entries {
			delete(menuLocations.entries, key)
			break
		}
	}
	menuLocations.entries[menuLocationKey{b, chat, message}] = menuLocation{callback, time.Now().Add(time.Hour)}
}
func (b *Bot) returnMenu(chat, message int64) string {
	menuLocations.Lock()
	defer menuLocations.Unlock()
	value, ok := menuLocations.entries[menuLocationKey{b, chat, message}]
	if ok && time.Now().Before(value.expires) {
		return value.callback
	}
	return "menu|ps|0"
}
func containerName(c MyType.Container) string {
	if len(c.Names) > 0 && c.Names[0] != "" {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return shortID(c.ID)
}
func compactName(s string) string {
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:23]) + "…"
	}
	return s
}
func containerButton(c MyType.Container) string {
	icon := "⚪"
	if strings.EqualFold(c.State, "running") {
		icon = "🟢"
	}
	if strings.EqualFold(c.State, "paused") {
		icon = "⏸"
	}
	label := icon + " " + compactName(containerName(c))
	if c.HostName != "" {
		label += " · " + compactName(c.HostName)
	}
	if c.Update {
		label += " ⬆"
	}
	return label
}
func addGridButton(rows *[][]telegram.InlineKeyboardButton, button telegram.InlineKeyboardButton, index int) {
	if index%2 == 0 {
		*rows = append(*rows, []telegram.InlineKeyboardButton{})
	}
	n := len(*rows) - 1
	(*rows)[n] = append((*rows)[n], button)
}
func pageBounds(total, page, size int) (int, int, int, int) {
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	start := page * size
	end := start + size
	if end > total {
		end = total
	}
	return page, pages, start, end
}

// renderContainerGrid 只在按钮显示状态、名称、更新；操作全部保留在详情面板。
func (b *Bot) renderContainerGrid(chatID int64, mode string, page int, messageID int64) {
	list, err := utiles.GetAllContainers(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 获取容器失败："+escapeHTML(err.Error()), mainMenuKeyboard())
		return
	}
	list = utiles.CheckImageUpdate(b.svcCtx, list)
	filtered := make([]MyType.Container, 0, len(list))
	for _, c := range list {
		running := strings.EqualFold(c.State, "running")
		if mode == "run" && !running || mode == "stopped" && running {
			continue
		}
		filtered = append(filtered, c)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].HostID != filtered[j].HostID {
			return filtered[i].HostID < filtered[j].HostID
		}
		return containerName(filtered[i]) < containerName(filtered[j])
	})
	page, pages, start, end := pageBounds(len(filtered), page, pageSize)
	title := map[string]string{"ps": "全部容器", "run": "运行中", "stopped": "已停止"}[mode]
	text := fmt.Sprintf("<b>📦 %s</b> · %d 个 · %d/%d\n🟢 运行　⚪ 停止　⬆ 可更新", title, len(filtered), page+1, pages)
	var rows [][]telegram.InlineKeyboardButton
	for i, c := range filtered[start:end] {
		addGridButton(&rows, telegram.InlineKeyboardButton{Text: containerButton(c), CallbackData: fmt.Sprintf("panel|%s|%s|%s", c.ID, containerName(c), encodeHostRef(c.HostID))}, i)
	}
	rows = append(rows, buildPager("menu|"+mode, page, pages, "menu|home")...)
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "全部", CallbackData: "menu|ps|0"}, {Text: "🟢 运行", CallbackData: "menu|run|0"}, {Text: "⚪ 停止", CallbackData: "menu|stopped|0"}})
	callback := fmt.Sprintf("menu|%s|%d", mode, page)
	if messageID == 0 {
		kb, err := b.encodeKeyboard(chatID, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
		if err != nil {
			return
		}
		messageID, err = b.client.SendMessageReturnID(chatID, text, kb)
		if err != nil {
			return
		}
	} else {
		b.editMessageKeyboard(chatID, messageID, text, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
	}
	b.rememberMenu(chatID, messageID, callback)
}
