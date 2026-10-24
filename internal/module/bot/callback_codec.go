package bot

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

const callbackTTL = 30 * time.Minute
const callbackCapacity = 20000

type callbackRecord struct {
	ChatID    int64
	Payload   string
	ExpiresAt time.Time
	Consumed  bool
}

// encodeHostRef 只在服务端保存完整实例身份，不向 Telegram 暴露可变排序码。
func encodeHostRef(hostID string) string {
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	return "h:" + base64.RawURLEncoding.EncodeToString([]byte(hostID))
}

// encodeKeyboard 深拷贝键盘；随机短令牌保证 callback_data 始终低于 64 字节。
func (b *Bot) encodeKeyboard(chatID int64, keyboard *telegram.InlineKeyboardMarkup) (*telegram.InlineKeyboardMarkup, error) {
	if keyboard == nil {
		return nil, nil
	}
	b.callbackMu.Lock()
	defer b.callbackMu.Unlock()
	if b.callbackStore == nil {
		b.callbackStore = make(map[string]callbackRecord)
	}
	now := time.Now()
	for token, record := range b.callbackStore {
		if !now.Before(record.ExpiresAt) {
			delete(b.callbackStore, token)
		}
	}
	count := 0
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData != "" {
				count++
			}
		}
	}
	// 不静默淘汰有效操作，容量用尽明确失败，避免按钮指向别的目标。
	if len(b.callbackStore)+count > callbackCapacity {
		return nil, fmt.Errorf("交互会话过多，请稍后重新打开菜单")
	}
	result := &telegram.InlineKeyboardMarkup{}
	for _, row := range keyboard.InlineKeyboard {
		copyRow := make([]telegram.InlineKeyboardButton, len(row))
		for i, button := range row {
			copyRow[i] = button
			if button.CallbackData == "" {
				continue
			}
			var random [18]byte
			if _, err := rand.Read(random[:]); err != nil {
				return nil, fmt.Errorf("生成安全按钮失败")
			}
			token := "cb1:" + base64.RawURLEncoding.EncodeToString(random[:])
			b.callbackStore[token] = callbackRecord{ChatID: chatID, Payload: button.CallbackData, ExpiresAt: now.Add(callbackTTL)}
			copyRow[i].CallbackData = token
		}
		result.InlineKeyboard = append(result.InlineKeyboard, copyRow)
	}
	return result, nil
}

// decodeCallback 拒绝旧的可变主机码与重启前按钮，仅允许旧菜单重新生成安全键盘。
func (b *Bot) decodeCallback(chatID int64, callback *telegram.CallbackQuery) (*telegram.CallbackQuery, error) {
	if callback == nil {
		return nil, fmt.Errorf("无效按钮")
	}
	result := *callback
	if !strings.HasPrefix(callback.Data, "cb1:") {
		if callback.Data == "menu|home" || callback.Data == "cancel" {
			return &result, nil
		}
		return nil, fmt.Errorf("按钮已过期，请发送 /menu 重新打开")
	}
	b.callbackMu.Lock()
	defer b.callbackMu.Unlock()
	record, ok := b.callbackStore[callback.Data]
	if !ok || record.ChatID != chatID || !time.Now().Before(record.ExpiresAt) {
		return nil, fmt.Errorf("按钮已过期，请发送 /menu 重新打开")
	}
	if record.Consumed {
		return nil, fmt.Errorf("操作已提交，请勿重复点击")
	}
	// 确认页最终提交一次性消费，列表、翻页和展示确认页可重复访问。
	if callbackIsSubmission(record.Payload) {
		record.Consumed = true
		b.callbackStore[callback.Data] = record
	}
	result.Data = record.Payload
	return &result, nil
}

func callbackIsSubmission(payload string) bool {
	parts := strings.Split(payload, "|")
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "confirm", "dotag", "cmpaconf":
		return true
	case "act":
		return len(parts) > 1 && parts[1] != "kill" && parts[1] != "remove" && parts[1] != "update"
	case "batchupdate":
		return len(parts) > 1 && parts[1] == "confirm"
	case "rres":
		return len(parts) > 1 && (parts[1] == "retrun" || parts[1] == "updrun")
	case "backup":
		return len(parts) > 1 && (parts[1] == "restore_confirm" || parts[1] == "delconf" || parts[1] == "create")
	case "settings":
		return len(parts) > 1 && parts[1] == "prune_exec"
	}
	return false
}
