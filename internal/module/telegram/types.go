package telegram

import "encoding/json"

// InputRichMessage 仅实现 Shell 所需的 HTML 分支；官方要求 html/markdown/blocks 三选一。
// 关闭自动实体检测，避免终端输出被自动转换为链接或命令。
type InputRichMessage struct {
	HTML                string `json:"html"`
	SkipEntityDetection bool   `json:"skip_entity_detection,omitempty"`
}

// Update 对应 Telegram 的一次更新。
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Message 文本消息。
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
	// 保留代发与转发来源标记，供 Shell 拒绝非本人直接发送的输入。
	SenderChat        *Chat           `json:"sender_chat,omitempty"`
	ForwardOrigin     json.RawMessage `json:"forward_origin,omitempty"`
	ForwardFrom       *User           `json:"forward_from,omitempty"`
	ForwardFromChat   *Chat           `json:"forward_from_chat,omitempty"`
	ForwardSenderName string          `json:"forward_sender_name,omitempty"`
	ForwardDate       int64           `json:"forward_date,omitempty"`
}

// CallbackQuery inline 按钮回调。
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// User 用户信息。
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Chat 会话信息。
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// InlineKeyboardMarkup inline 键盘。
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton inline 按钮，CallbackData 会随点击回传。
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// BotCommand 机器人命令菜单项（用于 setMyCommands）。
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// ChatID 返回消息或回调所属的会话ID。
func (u *Update) ChatID() int64 {
	if u.Message != nil && u.Message.Chat != nil {
		return u.Message.Chat.ID
	}
	if u.CallbackQuery != nil && u.CallbackQuery.Message != nil && u.CallbackQuery.Message.Chat != nil {
		return u.CallbackQuery.Message.Chat.ID
	}
	return 0
}
