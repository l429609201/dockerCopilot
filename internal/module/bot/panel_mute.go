package bot

import (
	"fmt"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// muteSettingsPageSize 更新通知设置面板每页显示的容器数（双列，即 5 行 × 2 列）。
const muteSettingsPageSize = 10

// sendMuteSettings 推送"更新通知设置"面板：容器双列排布、每页 10 个、支持上下页翻页。
// 已屏蔽显示 🔕、未屏蔽显示 🔔，点击就地切换（编辑原消息刷新，保持当前页）。
func (b *Bot) sendMuteSettings(chatID int64, page int, messageID int64) {
	// 汇总所有已启用 Docker 主机的容器（通知屏蔽设置覆盖远程主机）
	containers, err := utiles.GetAllContainers(b.svcCtx)
	if err != nil {
		b.reply(chatID, "❌ 获取容器列表失败："+err.Error())
		return
	}

	// 当前屏蔽集合
	cfg := b.svcCtx.AppConfig.Get()
	muted := make(map[string]struct{}, len(cfg.Telegram.MutedContainers))
	for _, n := range cfg.Telegram.MutedContainers {
		muted[n] = struct{}{}
	}

	// 先收集有效容器名（过滤空名），再分页
	var names []string
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	// 分页计算
	totalPages := (len(names) + muteSettingsPageSize - 1) / muteSettingsPageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * muteSettingsPageSize
	end := start + muteSettingsPageSize
	if end > len(names) {
		end = len(names)
	}

	// 双列排布：每两个容器占一行
	var rows [][]telegram.InlineKeyboardButton
	var pending []telegram.InlineKeyboardButton
	for _, name := range names[start:end] {
		icon := "🔔" // 未屏蔽：正常通知
		if _, ok := muted[name]; ok {
			icon = "🔕" // 已屏蔽
		}
		// 携带当前页码，切换后仍停留在本页；容器名可能含 | 放在末尾用 Join 还原
		btn := telegram.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", icon, name),
			CallbackData: fmt.Sprintf("mute|%d|%s", page, name),
		}
		pending = append(pending, btn)
		if len(pending) == 2 {
			rows = append(rows, pending)
			pending = nil
		}
	}
	if len(pending) > 0 {
		rows = append(rows, pending) // 落单的最后一个
	}

	// 分页导航
	if totalPages > 1 {
		var nav []telegram.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("menu|mute|%d", page-1)})
		}
		nav = append(nav, telegram.InlineKeyboardButton{Text: fmt.Sprintf("%d/%d", page+1, totalPages), CallbackData: fmt.Sprintf("menu|mute|%d", page)})
		if page < totalPages-1 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("menu|mute|%d", page+1)})
		}
		rows = append(rows, nav)
	}

	// 底部返回按钮
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})

	text := "<b>🔕 更新通知设置</b>\n\n点击容器切换是否推送「有更新」通知：\n🔔 正常通知　🔕 已屏蔽"
	if totalPages > 1 {
		text += fmt.Sprintf("\n\n第 %d/%d 页，共 %d 个容器", page+1, totalPages, len(names))
	}
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// setMuteState 切换指定容器的更新通知屏蔽状态（存在则移除、不存在则加入），不负责刷新界面。
// 供 toggleMute（设置面板）与 toggleMuteInPlace（详情页就地切换）复用。
func (b *Bot) setMuteState(name string) {
	_ = b.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		list := cfg.Telegram.MutedContainers
		idx := -1
		for i, n := range list {
			if n == name {
				idx = i
				break
			}
		}
		if idx >= 0 {
			// 已在屏蔽列表 → 移除（恢复通知）
			cfg.Telegram.MutedContainers = append(list[:idx], list[idx+1:]...)
		} else {
			// 不在 → 加入屏蔽
			cfg.Telegram.MutedContainers = append(list, name)
		}
		return nil
	})
}

// toggleMute 切换指定容器的更新通知屏蔽状态，并刷新「更新通知设置」面板。
func (b *Bot) toggleMute(chatID int64, name string, page int, messageID int64) {
	b.setMuteState(name)
	// 刷新面板（编辑原消息），保持在当前分页
	b.sendMuteSettings(chatID, page, messageID)
}

// toggleMuteInPlace 在「更新提醒详情页」就地切换通知屏蔽状态：
// 切换后不跳转到设置面板，而是重新渲染当前页详情（编辑本条消息），
// 让被屏蔽的容器按钮就地变为 🔕、可再次点击恢复。
func (b *Bot) toggleMuteInPlace(chatID int64, name string, page int, messageID int64) {
	b.setMuteState(name)
	b.resendUpdateNotification(chatID, page, messageID, false) // 始终回到未屏蔽列表
}
