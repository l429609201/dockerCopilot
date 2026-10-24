package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

// 回调测试仅使用内存键盘，不创建 Docker 或 Telegram 客户端。
func TestCallbackIdentityAndOneTimeSubmission(t *testing.T) {
	b := &Bot{}
	payload := "confirm|stop|full-id|" + strings.Repeat("长名称", 100) + "|" + encodeHostRef("remote-nas")
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "确认", CallbackData: payload}}}}
	encoded, err := b.encodeKeyboard(11, kb)
	if err != nil {
		t.Fatal(err)
	}
	token := encoded.InlineKeyboard[0][0].CallbackData
	if len(token) > 64 || kb.InlineKeyboard[0][0].CallbackData != payload {
		t.Fatal("回调超长或修改了源键盘")
	}
	cb := &telegram.CallbackQuery{Data: token}
	if _, err = b.decodeCallback(22, cb); err == nil {
		t.Fatal("不允许跨聊天使用按钮")
	}
	decoded, err := b.decodeCallback(11, cb)
	if err != nil || decoded.Data != payload {
		t.Fatalf("回调丢失原始身份: %v", err)
	}
	if _, err = b.decodeCallback(11, cb); err == nil {
		t.Fatal("重复确认必须拒绝")
	}
	if cb.Data != token {
		t.Fatal("不应修改原更新对象")
	}
}

func TestCallbackExpiryAndHostReference(t *testing.T) {
	b := &Bot{}
	for _, host := range []string{"local", "nas-2", "远程|实例"} {
		got := b.hostFromParts([]string{encodeHostRef(host)}, 0)
		if got != host {
			t.Fatalf("实例身份变化: %q", got)
		}
	}
	if b.hostFromParts([]string{"1"}, 0) == "local" {
		t.Fatal("旧排序码不得回落本地")
	}
	if _, err := b.decodeCallback(11, &telegram.CallbackQuery{Data: "act|stop|id|name|1"}); err == nil {
		t.Fatal("旧危险按钮应失效")
	}
	b.callbackStore = map[string]callbackRecord{"cb1:expired": {ChatID: 11, Payload: "menu|home", ExpiresAt: time.Now().Add(-time.Minute)}}
	if _, err := b.decodeCallback(11, &telegram.CallbackQuery{Data: "cb1:expired"}); err == nil {
		t.Fatal("已过期按钮应失效")
	}
}
