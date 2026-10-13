package telegram

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// 使用内存 RoundTripper 隔离请求，不连接 Telegram 或启动真实 Bot。
type fakeRoundTripper func(*http.Request) (*http.Response, error)

func (f fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func testClient(f fakeRoundTripper) *Client {
	return &Client{token: "123:secret", baseURL: "https://example.invalid/bot123:secret", httpClient: &http.Client{Transport: f}}
}

func TestEditNotModifiedIsSuccess(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return fakeResponse(400, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`), nil
	})
	if err := c.EditMessageText(1, 2, "<b>same</b>", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	if err := c.SendMessage(1, "same", nil); err == nil {
		t.Fatal("发送错误不应被忽略")
	}
}

func TestSendNetworkFailureNotRetriedAndRedacted(t *testing.T) {
	for _, document := range []bool{false, true} {
		calls := 0
		c := testClient(func(r *http.Request) (*http.Response, error) {
			calls++
			return nil, &url.Error{Op: "Post", URL: r.URL.String(), Err: io.EOF}
		})
		var err error
		if document {
			err = c.SendDocument(1, "log.txt", []byte("test"), "")
		} else {
			err = c.SendMessage(1, "hello", nil)
		}
		if err == nil || strings.Contains(err.Error(), "123:secret") || strings.Contains(err.Error(), "example.invalid") {
			t.Fatalf("不安全错误: %v", err)
		}
		if calls != 1 {
			t.Fatalf("非幂等请求不应重试: %d", calls)
		}
	}
}

func TestIdempotentNetworkRetry(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls < 2 {
			return nil, io.EOF
		}
		return fakeResponse(200, `{"ok":true,"result":true}`), nil
	})
	if err := c.EditMessageText(1, 2, "hello", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRateLimitRetryAndLongDelay(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return fakeResponse(429, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`), nil
		}
		return fakeResponse(200, `{"ok":true,"result":{"message_id":9}}`), nil
	})
	id, err := c.SendMessageReturnID(1, "hello", nil)
	if err != nil || id != 9 || calls != 2 {
		t.Fatalf("id=%d calls=%d err=%v", id, calls, err)
	}
	calls = 0
	c = testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return fakeResponse(429, `{"ok":false,"error_code":429,"description":"123:secret","parameters":{"retry_after":60}}`), nil
	})
	err = c.SendMessage(1, "hello", nil)
	if err == nil || !strings.Contains(err.Error(), "retry_after=60") || strings.Contains(err.Error(), "123:secret") || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestHTMLNotSilentlyTruncated(t *testing.T) {
	text := "<b>" + strings.Repeat("中文 &amp; ", 500) + "</b>"
	c := testClient(func(r *http.Request) (*http.Response, error) {
		var p map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p["text"] != text || p["parse_mode"] != "HTML" {
			t.Fatal("原文或HTML模式被修改")
		}
		return fakeResponse(400, `{"ok":false,"error_code":400,"description":"Bad Request: message is too long"}`), nil
	})
	if err := c.SendMessage(1, text, nil); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("超限必须显式报错: %v", err)
	}
}

// 验证编辑清旧按钮与发送省略键盘的语义保持区分。
func TestNilKeyboardEditClearsButSendOmits(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		markup, exists := payload["reply_markup"]
		if strings.HasSuffix(r.URL.Path, "/editMessageText") {
			if !exists || string(markup) != `{"inline_keyboard":[]}` {
				t.Fatalf("编辑必须发送空键盘数组: %s", markup)
			}
		} else if exists {
			t.Fatalf("发送 nil 键盘应继续省略: %s", markup)
		}
		return fakeResponse(200, `{"ok":true,"result":{"message_id":9}}`), nil
	})
	if err := c.EditMessageText(1, 2, "正在执行", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.SendMessage(1, "完成", nil); err != nil {
		t.Fatal(err)
	}
}

func TestRetryBudget(t *testing.T) {
	calls := 0
	c := testClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return fakeResponse(429, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`), nil
	})
	if err := c.SendDocument(1, "test.txt", []byte("test"), ""); err == nil {
		t.Fatal("重试耗尽必须返回错误")
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
}
