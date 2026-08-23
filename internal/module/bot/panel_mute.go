package bot

import (
	"fmt"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// sendMuteSettings 推送"更新通知设置"面板：逐个列出容器，
// 已屏蔽显示 🔕、未屏蔽显示 🔔，点击切换（编辑原消息刷新）。
func (b *Bot) sendMuteSettings(chatID int64, messageID int64) {
	containers, err := utiles.GetContainerList(b.svcCtx)
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

	var rows [][]telegram.InlineKeyboardButton
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			continue
		}
		icon := "🔔" // 未屏蔽：正常通知
		if _, ok := muted[name]; ok {
			icon = "🔕" // 已屏蔽
		}
		rows = append(rows, []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%s %s", icon, name), CallbackData: fmt.Sprintf("mute|%s", name)},
		})
	}
	// 底部返回按钮
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	})

	text := "<b>🔕 更新通知设置</b>\n\n点击容器切换是否推送「有更新」通知：\n🔔 正常通知　🔕 已屏蔽"
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, text, kb)
	} else {
		b.replyKeyboard(chatID, text, kb)
	}
}

// toggleMute 切换指定容器的更新通知屏蔽状态，并刷新面板。
func (b *Bot) toggleMute(chatID int64, name string, messageID int64) {
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
	// 刷新面板（编辑原消息）
	b.sendMuteSettings(chatID, messageID)
}
