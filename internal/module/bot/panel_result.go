package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/notify"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/zeromicro/go-zero/core/logx"
)

// ruleResultPageSize 明细列表每页展示的容器数量（每个容器占一行按钮）。
const ruleResultPageSize = 8

// sendRuleResultSummary 发送/编辑定时更新完成摘要：
// 正文只展示统计数字 + 已更新容器列表；跳过/失败改由底部内联按钮按需查看。
// messageID > 0 时编辑原消息，否则发送新消息（周期通知场景）。
func (b *Bot) sendRuleResultSummary(chatID int64, res *notify.RuleUpdateResult, messageID int64) {
	if res == nil {
		return
	}
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>✅ 定时更新完成</b>\n规则「%s」执行完成\n\n", escapeHTML(res.RuleName)))
	text.WriteString(fmt.Sprintf("📊 统计：更新 %d 个，跳过 %d 个，失败 %d 个\n",
		len(res.Updated), len(res.Skipped), len(res.Failed)))

	// 正文只铺开"已更新"列表（用户最关心的成功项）。最多展示 30 条，避免超 TG 消息长度上限。
	if len(res.Updated) > 0 {
		text.WriteString("\n✅ 已更新：\n")
		const maxShow = 30
		for i, item := range res.Updated {
			if i >= maxShow {
				text.WriteString(fmt.Sprintf("  … 还有 %d 个\n", len(res.Updated)-maxShow))
				break
			}
			text.WriteString(fmt.Sprintf("  • %s（%s）\n", escapeHTML(item.Name), escapeHTML(item.Reason)))
		}
	} else {
		text.WriteString("\n本次没有容器被更新。")
	}

	// 底部按需出现按钮：查看跳过 / 查看失败 / 重试全部失败
	var rows [][]telegram.InlineKeyboardButton
	var navRow []telegram.InlineKeyboardButton
	if len(res.Skipped) > 0 {
		navRow = append(navRow, telegram.InlineKeyboardButton{
			Text:         fmt.Sprintf("⏭️ 查看跳过 %d", len(res.Skipped)),
			CallbackData: fmt.Sprintf("rres|skip|%s|0", res.RuleID),
		})
	}
	if len(res.Failed) > 0 {
		navRow = append(navRow, telegram.InlineKeyboardButton{
			Text:         fmt.Sprintf("❌ 查看失败 %d", len(res.Failed)),
			CallbackData: fmt.Sprintf("rres|fail|%s|0", res.RuleID),
		})
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}
	if len(res.Failed) > 0 {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         fmt.Sprintf("🔁 重试全部失败 (%d)", len(res.Failed)),
			CallbackData: fmt.Sprintf("rres|retry|%s", res.RuleID),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{
		Text: "⬅ 返回主菜单", CallbackData: "menu|home",
	}})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// sendRuleResultDetail 展开某规则的"跳过"或"失败"明细（编辑原消息）。
// kind: "skip" 或 "fail"；每个容器一行带 [🔄 更新] 按钮，底部翻页 + 返回摘要。
func (b *Bot) sendRuleResultDetail(chatID int64, ruleID, kind string, page int, messageID int64) {
	res := notify.GetRuleUpdateResult(ruleID)
	if res == nil {
		b.editOrReplyKeyboard(chatID, messageID, "⚠️ 该执行结果已过期（服务可能已重启），请重新触发规则。",
			&telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
			}}})
		return
	}

	var items []notify.ResultItem
	var title string
	if kind == "fail" {
		items, title = res.Failed, "❌ 更新失败明细"
	} else {
		items, title = res.Skipped, "⏭️ 已跳过明细"
	}
	if len(items) == 0 {
		b.sendRuleResultSummary(chatID, res, messageID)
		return
	}

	totalPages := (len(items) + ruleResultPageSize - 1) / ruleResultPageSize
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * ruleResultPageSize
	end := start + ruleResultPageSize
	if end > len(items) {
		end = len(items)
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>%s</b>\n规则「%s」", title, escapeHTML(res.RuleName)))
	if totalPages > 1 {
		text.WriteString(fmt.Sprintf("（第 %d/%d 页）", page+1, totalPages))
	}
	text.WriteString("：\n\n")

	var rows [][]telegram.InlineKeyboardButton
	for i := start; i < end; i++ {
		item := items[i]
		text.WriteString(fmt.Sprintf("%d. <b>%s</b>\n   %s\n", i+1, escapeHTML(item.Name), escapeHTML(item.Reason)))
		// 每个容器一行「更新」按钮：rres|upd|<ruleID>|<kind>|<全局索引>，回调时按索引取容器信息更新
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         fmt.Sprintf("🔄 更新 %s", truncName(item.Name, 20)),
			CallbackData: fmt.Sprintf("rres|upd|%s|%s|%d", ruleID, kind, i),
		}})
	}

	// 翻页行（多页时）
	if totalPages > 1 {
		var nav []telegram.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "⬅ 上一页", CallbackData: fmt.Sprintf("rres|%s|%s|%d", kind, ruleID, page-1)})
		}
		if page < totalPages-1 {
			nav = append(nav, telegram.InlineKeyboardButton{Text: "下一页 ➡", CallbackData: fmt.Sprintf("rres|%s|%s|%d", kind, ruleID, page+1)})
		}
		if len(nav) > 0 {
			rows = append(rows, nav)
		}
	}
	// 失败明细页追加"重试全部失败"
	if kind == "fail" {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text: fmt.Sprintf("🔁 重试全部失败 (%d)", len(items)), CallbackData: fmt.Sprintf("rres|retry|%s", ruleID),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{
		Text: "⬅ 返回", CallbackData: fmt.Sprintf("rres|sum|%s", ruleID),
	}})

	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// truncName 截断容器名用于按钮文本，避免按钮过宽（UTF-8 安全）。
func truncName(name string, max int) string {
	r := []rune(name)
	if len(r) <= max {
		return name
	}
	return string(r[:max-1]) + "…"
}

// parsePageArg 解析回调里的页码参数，非法返回 0。
func parsePageArg(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// updateResultItem 更新明细列表中的单个容器（跳过/失败明细页点「更新」触发）。
// kind+idx 定位到 result store 里的具体条目，按其 HostID 路由更新并在原消息显示进度。
func (b *Bot) updateResultItem(chatID int64, ruleID, kind string, idx int, messageID int64) {
	res := notify.GetRuleUpdateResult(ruleID)
	if res == nil {
		b.editMessage(chatID, messageID, "⚠️ 该执行结果已过期（服务可能已重启），请重新触发规则。")
		return
	}
	var items []notify.ResultItem
	if kind == "fail" {
		items = res.Failed
	} else {
		items = res.Skipped
	}
	if idx < 0 || idx >= len(items) {
		b.editMessage(chatID, messageID, "⚠️ 条目不存在，可能已变更，请返回重试。")
		return
	}
	item := items[idx]
	// 复用 doUpdate：按 hostID 路由、编辑本消息持续显示更新进度
	b.doUpdate(chatID, item.ID, item.Name, item.HostID, messageID)
}

// confirmRetryFailed 二次确认「重试全部失败」。
func (b *Bot) confirmRetryFailed(chatID int64, ruleID string, messageID int64) {
	res := notify.GetRuleUpdateResult(ruleID)
	if res == nil || len(res.Failed) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "✅ 没有需要重试的失败项。",
			&telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
				{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
			}}})
		return
	}
	var text strings.Builder
	text.WriteString(fmt.Sprintf("⚠️ <b>确认重试全部失败</b>\n\n将重新提交以下 %d 个容器的更新：\n\n", len(res.Failed)))
	for i, item := range res.Failed {
		if i >= 15 {
			text.WriteString(fmt.Sprintf("… 还有 %d 个\n", len(res.Failed)-15))
			break
		}
		text.WriteString(fmt.Sprintf("• %s\n", escapeHTML(item.Name)))
	}
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "✅ 确认重试", CallbackData: fmt.Sprintf("rres|retrun|%s", ruleID)}},
		{{Text: "⬅ 返回", CallbackData: fmt.Sprintf("rres|sum|%s", ruleID)}},
	}}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), kb)
}

// executeRetryFailed 批量重新提交全部失败容器的更新任务（按各自 HostID 路由）。
func (b *Bot) executeRetryFailed(chatID int64, ruleID string, messageID int64) {
	backHomeKb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "⬅ 返回主菜单", CallbackData: "menu|home"},
	}}}
	res := notify.GetRuleUpdateResult(ruleID)
	if res == nil || len(res.Failed) == 0 {
		b.editOrReplyKeyboard(chatID, messageID, "✅ 没有需要重试的失败项。", backHomeKb)
		return
	}
	b.editOrReplyKeyboard(chatID, messageID, fmt.Sprintf("🚀 开始重试 %d 个失败容器...", len(res.Failed)), nil)

	success := 0
	var failNames []string
	for _, item := range res.Failed {
		// 按容器所属主机提交更新任务（沿用其原镜像）
		taskID, err := containerops.NewForHost(b.svcCtx, item.HostID).Update(item.ID, item.Name, item.Image)
		if err != nil {
			failNames = append(failNames, fmt.Sprintf("• %s：%s", item.Name, err.Error()))
			continue
		}
		success++
		logx.Infof("重试失败更新：容器 %s 任务已提交，taskID=%s", item.Name, taskID)
	}

	var text strings.Builder
	text.WriteString("<b>📊 重试提交结果</b>\n\n")
	text.WriteString(fmt.Sprintf("✅ 成功提交：%d 个\n", success))
	if len(failNames) > 0 {
		text.WriteString(fmt.Sprintf("❌ 提交失败：%d 个\n\n", len(failNames)))
		for _, s := range failNames {
			text.WriteString(escapeHTML(s) + "\n")
		}
	} else {
		text.WriteString("\n💡 更新任务已在后台执行，可用 /ps 查看状态。")
	}
	b.editOrReplyKeyboard(chatID, messageID, text.String(), backHomeKb)
}
