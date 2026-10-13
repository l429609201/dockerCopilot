package telegram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Client 是零依赖的 Telegram Bot API 客户端，基于标准库 net/http。
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient 创建客户端。proxy 为空时不使用代理。
func NewClient(token, proxy string) (*Client, error) {
	transport := &http.Transport{}
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址失败，请检查代理配置")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &Client{
		token:      token,
		httpClient: &http.Client{Transport: transport, Timeout: 65 * time.Second},
		baseURL:    "https://api.telegram.org/bot" + token,
	}, nil
}

// apiResponse 是 Telegram API 的通用响应包裹。
type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// apiError 保存已脱敏的结构化 API 错误，网络错误不会被判为接口不支持。
type apiError struct {
	method      string
	httpStatus  int
	code        int
	description string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("telegram api error (HTTP %d, code %d): %s", e.httpStatus, e.code, e.description)
}

// IsRichMessageUnsupported 仅识别明确能力拒绝，防止网络失败后降级重复发送。
// 泛化 404 可能是无效 token，不能作为不支持依据。
func IsRichMessageUnsupported(err error) bool {
	var e *apiError
	if !errors.As(err, &e) || (e.method != "sendRichMessage" && e.method != "editMessageText") {
		return false
	}
	if (e.httpStatus != 400 && e.httpStatus != 404) || (e.code != 400 && e.code != 404) {
		return false
	}
	description := strings.ToLower(e.description)
	// 旧接口忽略 rich_message 后会明确报告缺少 text；只有编辑请求允许据此降级。
	if e.method == "editMessageText" && e.code == 400 && e.httpStatus == 400 &&
		(description == "bad request: message text is empty" || description == "bad request: text must be non-empty") {
		return true
	}
	for _, phrase := range []string{
		"method not found", "unknown method", "method is not supported", "method not supported",
		"rich_message is not supported", "rich_message not supported", "rich messages are not supported",
		"rich messages not supported", "unknown field rich_message", "unknown parameter rich_message",
		"unsupported parameter rich_message", "unsupported field rich_message",
	} {
		if strings.Contains(description, phrase) {
			return true
		}
	}
	return false
}

// call 保留原有接口；不截断 HTML，格式及长度错误由 Telegram 明确返回给上层。
func (c *Client) call(method string, payload interface{}, out interface{}) error {
	var data []byte
	if payload != nil {
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("Telegram 请求编码失败")
		}
	}
	return c.request(method, "application/json", data, out)
}

// safeTransportError 不输出包含 Bot token 的请求 URL。
func (c *Client) safeTransportError(err error) error {
	// http.Client 可能再次包装 RoundTripper 返回的 url.Error，逐层去除请求地址。
	for {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) || urlErr.Err == nil {
			break
		}
		err = urlErr.Err
	}
	return fmt.Errorf("Telegram 请求失败: %s", c.redact(err.Error()))
}

func (c *Client) redact(text string) string {
	if c.token != "" {
		text = strings.ReplaceAll(text, c.token, "[REDACTED]")
		text = strings.ReplaceAll(text, url.QueryEscape(c.token), "[REDACTED]")
	}
	return text
}

// request 最多三次尝试；发送消息或文件遇到网络错误可能已经送达，绝不自动重发。
// 仅明确拒绝的 429 允许重发；超过五秒的等待交还上层，避免长期阻塞入站处理。
func (c *Client) request(method, contentType string, data []byte, out interface{}) error {
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+"/"+method, bytes.NewReader(data))
		if err != nil {
			return c.safeTransportError(err)
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			var temporary interface{ Timeout() bool }
			retryable := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || (errors.As(err, &temporary) && temporary.Timeout())
			if attempt < 2 && retryable && (method == "getUpdates" || method == "editMessageText" || method == "setMyCommands") {
				time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
				continue
			}
			return c.safeTransportError(err)
		}
		var apiResp apiResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("Telegram 响应解析失败 (HTTP %d)", resp.StatusCode)
		}
		if !apiResp.OK || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if method == "editMessageText" && (apiResp.ErrorCode == 400 || resp.StatusCode == 400) && strings.Contains(strings.ToLower(apiResp.Description), "message is not modified") {
				return nil
			}
			if apiResp.ErrorCode == 429 || resp.StatusCode == 429 {
				delay := apiResp.Parameters.RetryAfter
				if attempt < 2 && delay > 0 && delay <= 5 {
					time.Sleep(time.Duration(delay) * time.Second)
					continue
				}
				return fmt.Errorf("Telegram 限流 (429)，retry_after=%d 秒: %s", delay, c.redact(apiResp.Description))
			}
			return &apiError{method: method, httpStatus: resp.StatusCode, code: apiResp.ErrorCode, description: c.redact(apiResp.Description)}
		}
		if out != nil && len(apiResp.Result) > 0 {
			if err := json.Unmarshal(apiResp.Result, out); err != nil {
				return fmt.Errorf("Telegram 返回结果解析失败")
			}
		}
		return nil
	}
	return fmt.Errorf("Telegram 请求重试次数已耗尽")
}

// GetUpdates 长轮询获取更新。offset 为已确认的最大 update_id+1，timeout 为长轮询秒数。
func (c *Client) GetUpdates(offset, timeout int) ([]Update, error) {
	payload := map[string]interface{}{
		"offset":  offset,
		"timeout": timeout,
	}
	var updates []Update
	if err := c.call("getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// SendMessage 发送文本消息，支持可选 inline keyboard。
func (c *Client) SendMessage(chatID int64, text string, keyboard *InlineKeyboardMarkup) error {
	_, err := c.SendMessageReturnID(chatID, text, keyboard)
	return err
}

// SendMessageReturnID 发送文本消息并返回新消息的 message_id。
// 用于需要后续编辑/删除该消息的场景（如交互式 Shell 会话的"终端消息"）。
func (c *Client) SendMessageReturnID(chatID int64, text string, keyboard *InlineKeyboardMarkup) (int64, error) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	var msg Message
	if err := c.call("sendMessage", payload, &msg); err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

// SendRichMessageReturnID 使用独立的富消息接口发送 HTML，不改变普通消息语义。
// html 由调用方转义并限制长度；仅关闭终端内容的自动实体检测，不截断原文。
func (c *Client) SendRichMessageReturnID(chatID int64, html string, keyboard *InlineKeyboardMarkup) (int64, error) {
	payload := map[string]interface{}{
		"chat_id":      chatID,
		"rich_message": InputRichMessage{HTML: html, SkipEntityDetection: true},
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	var msg Message
	if err := c.call("sendRichMessage", payload, &msg); err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

// EditRichMessage 通过 editMessageText.rich_message 原地更新，不发送 text/parse_mode。
// nil 键盘显式清旧按钮，避免终端结束后仍残留可操作按钮。
func (c *Client) EditRichMessage(chatID, messageID int64, html string, keyboard *InlineKeyboardMarkup) error {
	if keyboard == nil {
		keyboard = &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}
	}
	return c.call("editMessageText", map[string]interface{}{
		"chat_id":      chatID,
		"message_id":   messageID,
		"rich_message": InputRichMessage{HTML: html, SkipEntityDetection: true},
		"reply_markup": keyboard,
	}, nil)
}

// DeleteMessage 删除指定消息。私聊中 Bot 可删除收到的消息（48 小时内）。
func (c *Client) DeleteMessage(chatID, messageID int64) error {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.call("deleteMessage", payload, nil)
}

// SetMyCommands 设置机器人命令菜单，客户端"/"或菜单键会展示这些命令。
func (c *Client) SetMyCommands(commands []BotCommand) error {
	return c.call("setMyCommands", map[string]interface{}{"commands": commands}, nil)
}

// EditMessageText 编辑已发送消息的文本与键盘（用于按钮操作后原地刷新）。
func (c *Client) EditMessageText(chatID, messageID int64, text string, keyboard *InlineKeyboardMarkup) error {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	// 编辑时 nil 表示清空键盘，不能省略该字段，否则旧操作按钮仍可能可点。
	if keyboard == nil {
		keyboard = &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}
	}
	payload["reply_markup"] = keyboard
	return c.call("editMessageText", payload, nil)
}

// SendDocument 以 multipart/form-data 上传文件作为文档发送（用于下载日志等场景）。
//   - filename：文档文件名（客户端显示与保存的名字）
//   - content：文件二进制内容
//   - caption：可选的文档说明文字（支持 HTML）
func (c *Client) SendDocument(chatID int64, filename string, content []byte, caption string) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	// 写入基础字段
	_ = writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		_ = writer.WriteField("caption", caption)
		_ = writer.WriteField("parse_mode", "HTML")
	}
	// 写入文件部分
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return c.request("sendDocument", writer.FormDataContentType(), buf.Bytes(), nil)
}

// AnswerCallbackQuery 应答一次回调查询，消除按钮 loading 状态。
func (c *Client) AnswerCallbackQuery(callbackID, text string) error {
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
	}
	return c.call("answerCallbackQuery", payload, nil)
}
