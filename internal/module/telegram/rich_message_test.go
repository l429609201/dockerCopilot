package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// 富消息契约只经内存传输验证，不访问真实 Telegram。
func TestRichMessagePayload(t *testing.T) {
	html := "<h2>终端</h2><pre>" + strings.Repeat("中文 &amp; ", 600) + "</pre><tg-button type=\"callback_data\" data=\"shell:stop\">停止</tg-button>"
	keyboard := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "停止", CallbackData: "shell:stop"}}}}
	for _, edit := range []bool{false, true} {
		for _, keys := range []*InlineKeyboardMarkup{nil, keyboard} {
			c := testClient(func(r *http.Request) (*http.Response, error) {
				method := "sendRichMessage"
				if edit {
					method = "editMessageText"
				}
				if !strings.HasSuffix(r.URL.Path, "/"+method) || r.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("错误请求: %s", r.URL.Path)
				}
				var p map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					t.Fatal(err)
				}
				if string(p["chat_id"]) != "42" || (edit && string(p["message_id"]) != "9") {
					t.Fatalf("目标不匹配: %s", p)
				}
				for _, key := range []string{"text", "parse_mode", "entities", "html", "features"} {
					if _, exists := p[key]; exists {
						t.Fatalf("不应携带 %s", key)
					}
				}
				var rich map[string]interface{}
				if err := json.Unmarshal(p["rich_message"], &rich); err != nil {
					t.Fatal(err)
				}
				if len(rich) != 2 || rich["html"] != html || rich["skip_entity_detection"] != true {
					t.Fatalf("富消息契约不匹配: %v", rich)
				}
				if keys != nil {
					want, _ := json.Marshal(keys)
					if string(p["reply_markup"]) != string(want) {
						t.Fatal("键盘未保留")
					}
				} else if edit {
					if string(p["reply_markup"]) != `{"inline_keyboard":[]}` {
						t.Fatal("编辑必须清空旧键盘")
					}
				} else if _, exists := p["reply_markup"]; exists {
					t.Fatal("发送 nil 键盘必须省略")
				}
				return fakeResponse(200, `{"ok":true,"result":{"message_id":9}}`), nil
			})
			if edit {
				if err := c.EditRichMessage(42, 9, html, keys); err != nil {
					t.Fatal(err)
				}
			} else if id, err := c.SendRichMessageReturnID(42, html, keys); err != nil || id != 9 {
				t.Fatalf("id=%d err=%v", id, err)
			}
		}
	}
}

func TestRichMessageUnsupported(t *testing.T) {
	for _, tc := range []struct {
		description string
		status      int
		edit        bool
		want        bool
	}{
		{"Bad Request: method not found", 400, false, true},
		{"Not Found: unknown method", 404, false, true},
		{"Bad Request: rich_message is not supported", 400, true, true},
		{"Bad Request: rich messages are not supported", 400, true, true},
		{"Bad Request: unknown field rich_message", 400, true, true},
		{"Bad Request: message text is empty", 400, true, true},
		{"Bad Request: text must be non-empty", 400, true, true},
		{"Bad Request: message text is empty", 400, false, false},
		{"Not Found", 404, false, false},
		{"Unauthorized", 401, false, false},
		{"Bad Request: can't parse rich_message HTML", 400, false, false},
		{"Bad Request: unsupported start tag in rich_message", 400, false, false},
		{"Bad Request: message to edit not found", 400, true, false},
		{"Bad Request: message is too long", 400, false, false},
		{"method not found", 500, false, false},
		{"method not found", 429, false, false},
	} {
		t.Run(tc.description+fmt.Sprint(tc.status), func(t *testing.T) {
			c := testClient(func(r *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]interface{}{"ok": false, "error_code": tc.status, "description": tc.description})
				return fakeResponse(tc.status, string(body)), nil
			})
			var err error
			if tc.edit {
				err = c.EditRichMessage(1, 2, "<p>test</p>", nil)
			} else {
				_, err = c.SendRichMessageReturnID(1, "<p>test</p>", nil)
			}
			if err == nil || IsRichMessageUnsupported(fmt.Errorf("wrapped: %w", err)) != tc.want {
				t.Fatalf("unsupported=%v want=%v err=%v", IsRichMessageUnsupported(err), tc.want, err)
			}
		})
	}
	if IsRichMessageUnsupported(nil) || IsRichMessageUnsupported(errors.New("method not found")) {
		t.Fatal("不能从非 API 错误文本推断接口不支持")
	}
}

func TestRichMessageNetworkFailureAndRedaction(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("123:secret: %w", io.EOF)
	})
	_, err := c.SendRichMessageReturnID(1, "<p>test</p>", nil)
	if err == nil || calls != 1 || IsRichMessageUnsupported(err) || strings.Contains(err.Error(), "123:secret") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	c = testClient(func(r *http.Request) (*http.Response, error) {
		return fakeResponse(400, `{"ok":false,"error_code":400,"description":"rich_message is not supported 123:secret"}`), nil
	})
	_, err = c.SendRichMessageReturnID(1, "<p>test</p>", nil)
	if err == nil || strings.Contains(err.Error(), "123:secret") || !IsRichMessageUnsupported(err) {
		t.Fatalf("API 错误应脱敏且保留分类: %v", err)
	}
}

// 复现线上明确的内容拒绝：可降级，但不能归为接口能力缺失。
func TestRichContentRequiredFallback(t *testing.T) {
	for _, method := range []string{"editMessageText", "sendRichMessage"} {
		err := &apiError{method: method, httpStatus: 400, code: 400, description: "Bad Request: RICH_MESSAGE_CONTENT_REQUIRED"}
		if !CanFallbackRichMessage(err) || IsRichMessageUnsupported(err) {
			t.Fatal("明确内容拒绝应允许回退且不误判接口能力")
		}
		for _, code := range []int{401, 429, 500} {
			err.code, err.httpStatus = code, code
			if CanFallbackRichMessage(err) {
				t.Fatalf("错误状态 %d 不应降级", code)
			}
		}
	}
	if CanFallbackRichMessage(errors.New("Bad Request: RICH_MESSAGE_CONTENT_REQUIRED")) || CanFallbackRichMessage(nil) {
		t.Fatal("非结构化API错误不能触发回退")
	}
}

func TestRichEditNotModified(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		return fakeResponse(400, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`), nil
	})
	if err := c.EditRichMessage(1, 2, "<p>same</p>", nil); err != nil {
		t.Fatal(err)
	}
}
