package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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
			return nil, fmt.Errorf("解析代理地址失败: %w", err)
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
}

// call 执行一次 API 调用并解析 result 到 out。
func (c *Client) call(method string, payload interface{}, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/"+method, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram api error: %s", apiResp.Description)
	}
	if out != nil && len(apiResp.Result) > 0 {
		return json.Unmarshal(apiResp.Result, out)
	}
	return nil
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
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
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

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/sendDocument", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram api error: %s", apiResp.Description)
	}
	return nil
}

// AnswerCallbackQuery 应答一次回调查询，消除按钮 loading 状态。
func (c *Client) AnswerCallbackQuery(callbackID, text string) error {
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
	}
	return c.call("answerCallbackQuery", payload, nil)
}
