package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/zeromicro/go-zero/core/logx"
)

const shellOutputBudget = 64 * 1024
const shellIdleTimeout = 15 * time.Minute

// shellSession 固定绑定发起者、聊天、完整容器与消息；所有可变状态由 mu 保护。
type shellSession struct {
	mu                           sync.Mutex
	renderMu                     sync.Mutex
	id, name, hostID, token      string
	chatID, ownerID, resultMsgID int64
	terminal                     io.ReadWriteCloser
	cancel                       context.CancelFunc
	done                         chan struct{}
	input                        chan shellInput
	closed                       bool
	output                       terminalOutput
	history                      []string
	showHistory                  bool
	lastActive                   time.Time
	lastShown                    string
	keyboard                     *telegram.InlineKeyboardMarkup
	client                       *telegram.Client
	plain                        bool
}
type shellInput struct {
	data      string
	messageID int64
	history   bool
}

// terminalOutput 是行式转录，不模拟 vim 等全屏应用；跨块保留转义状态与 UTF-8 尾部。
type terminalOutput struct {
	text    string
	pending []byte
	state   byte
}

func (o *terminalOutput) append(p []byte) {
	p = append(o.pending, p...)
	o.pending = nil
	var out strings.Builder
	for len(p) > 0 {
		if !utf8.FullRune(p) {
			o.pending = append([]byte(nil), p...)
			break
		}
		r, n := utf8.DecodeRune(p)
		p = p[n:]
		switch o.state {
		case 1:
			switch r {
			case '[':
				o.state = 2
			case ']', 'P', '^', '_':
				o.state = 3
			default:
				o.state = 0
			}
			continue
		case 2:
			if r >= 0x40 && r <= 0x7e {
				o.state = 0
			}
			continue
		case 3:
			if r == 7 {
				o.state = 0
			}
			if r == 27 {
				o.state = 4
			}
			continue
		case 4:
			if r == '\\' {
				o.state = 0
			} else {
				o.state = 3
			}
			continue
		}
		if r == 27 {
			o.state = 1
			continue
		}
		if r == '\r' {
			continue
		}
		if r == '\n' || r == '\t' || r >= 32 && r != 127 && !(r >= 0x80 && r <= 0x9f) {
			out.WriteRune(r)
		}
	}
	o.text = utf8Tail(o.text+out.String(), shellOutputBudget)
}
func utf8Tail(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	s = s[len(s)-budget:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
func shellMessageOwner(m *telegram.Message) int64 {
	if m == nil || m.From == nil || m.From.IsBot || m.SenderChat != nil || (len(m.ForwardOrigin) > 0 && string(m.ForwardOrigin) != "null") || m.ForwardFrom != nil || m.ForwardFromChat != nil || m.ForwardSenderName != "" || m.ForwardDate != 0 {
		return 0
	}
	return m.From.ID
}
func shellCallbackOwner(cb *telegram.CallbackQuery) int64 {
	if cb == nil || cb.From == nil || cb.From.IsBot {
		return 0
	}
	return cb.From.ID
}
func (s *shellSession) authorized(chatID, userID, messageID int64, token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && userID > 0 && s.chatID == chatID && s.ownerID == userID && s.resultMsgID == messageID && messageID > 0 && s.token == token
}
func (s *shellSession) close() bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.closed = true
	close(s.done)
	t := s.terminal
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if t != nil {
		_ = t.Close()
	}
	return true
}
func (b *Bot) removeShell(s *shellSession) bool {
	closed := s.close()
	b.shellMu.Lock()
	if b.shells[s.chatID] == s {
		delete(b.shells, s.chatID)
	}
	b.shellMu.Unlock()
	return closed
}
func (b *Bot) stopShells() {
	b.shellMu.Lock()
	b.shellStopped = true
	sessions := b.shells
	b.shells = make(map[int64]*shellSession)
	b.shellMu.Unlock()
	for _, s := range sessions {
		s.close()
	}
}

// promptExec 先预留聊天再后台连接，慢 Docker/TG 请求不占用长轮询。
func (b *Bot) promptExec(chatID int64, id, name, hostID string, messageID, userID int64) {
	if userID <= 0 {
		b.reply(chatID, "⛔ 无法验证终端发起者身份。")
		return
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return
	}
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	s := &shellSession{id: id, name: name, hostID: hostID, token: hex.EncodeToString(nonce[:]), chatID: chatID, ownerID: userID, resultMsgID: messageID, done: make(chan struct{}), input: make(chan shellInput, 32), lastActive: time.Now(), client: client}
	b.shellMu.Lock()
	if b.shellStopped || b.shells[chatID] != nil || len(b.shells) >= 16 {
		b.shellMu.Unlock()
		b.reply(chatID, "⚠️ 此聊天已有终端或全局终端已满（最多16个），请先关闭现有会话。")
		return
	}
	if b.shells == nil {
		b.shells = make(map[int64]*shellSession)
	}
	b.shells[chatID] = s
	b.shellMu.Unlock()
	go b.startShell(s)
}
func (b *Bot) startShell(s *shellSession) {
	// 面板已解析完整容器ID；TTY层再次核验，避免无超时列表查询阻塞建立。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return
	}
	s.cancel = cancel
	s.mu.Unlock()
	terminal, err := containerops.NewForHost(b.svcCtx, s.hostID).OpenTerminal(ctx, s.id, "sh")
	cancel()
	if err != nil {
		b.removeShell(s)
		b.reply(s.chatID, "❌ 无法开启容器终端，请检查容器状态和 Shell 是否可用。")
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = terminal.Close()
		return
	}
	s.terminal = terminal
	s.mu.Unlock()
	kb, err := b.encodeKeyboard(s.chatID, shellKeyboard(s.token))
	if err != nil {
		b.removeShell(s)
		return
	}
	s.mu.Lock()
	s.keyboard = kb
	s.mu.Unlock()
	if err = b.renderShell(s, true); err != nil {
		// 界面初始化失败必须可见，避免用户点击后终端无声关闭。
		logx.Errorf("Telegram Shell 初始化界面失败: %v", err)
		b.removeShell(s)
		b.reply(s.chatID, "❌ 终端界面初始化失败，请检查服务日志中的 Telegram Shell 错误。")
		return
	}
	go b.shellReadLoop(s, terminal)
	go b.shellWriteLoop(s, terminal)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			b.renewShellKeyboard(s)
			s.mu.Lock()
			expired := time.Since(s.lastActive) >= shellIdleTimeout
			s.mu.Unlock()
			if expired {
				b.finishShell(s.chatID, s)
				return
			}
			_ = b.renderShell(s, false)
		}
	}
}
func (b *Bot) shellReadLoop(s *shellSession, t io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := t.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.output.append(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			b.finishShell(s.chatID, s)
			return
		}
	}
}
func (b *Bot) shellWriteLoop(s *shellSession, t io.Writer) {
	for {
		select {
		case <-s.done:
			return
		case in := <-s.input:
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			n, err := io.WriteString(t, in.data)
			if err != nil || n != len(in.data) {
				b.finishShell(s.chatID, s)
				return
			}
			s.mu.Lock()
			if in.history {
				s.history = append(s.history, utf8Tail(strings.TrimSuffix(in.data, "\n"), 4096))
				if len(s.history) > 20 {
					s.history = s.history[len(s.history)-20:]
				}
			}
			s.mu.Unlock()
			// 只有完整写入成功才删除用户输入；失败仍可重试。
			if in.messageID > 0 {
				_ = s.client.DeleteMessage(s.chatID, in.messageID)
			}
		}
	}
}
func (b *Bot) enqueueShell(s *shellSession, in shellInput) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.input <- in:
		s.lastActive = time.Now()
		s.showHistory = false
		return true
	default:
		return false
	}
}
func (b *Bot) runShellCommand(chatID int64, s *shellSession, text string, messageID int64) {
	// 非文本消息没有隐含 Enter，保留空格等有效原始输入。
	if text == "" {
		return
	}
	if len(text) > shellOutputBudget || !b.enqueueShell(s, shellInput{data: text + "\n", messageID: messageID, history: true}) {
		b.reply(chatID, "⚠️ 终端输入队列已满或会话已结束，消息尚未执行。")
	}
}
func shellKeyboard(token string) *telegram.InlineKeyboardMarkup {
	rows := [][]telegram.InlineKeyboardButton{}
	for _, items := range [][][2]string{
		{{"Ctrl+C", "interrupt"}, {"Tab", "tab"}, {"Enter", "enter"}, {"Esc", "esc"}},
		{{"↑", "up"}, {"↓", "down"}, {"←", "left"}, {"→", "right"}},
		{{"📜 历史", "history"}, {"终端", "output"}, {"清屏", "clear"}, {"关闭", "close"}},
	} {
		row := []telegram.InlineKeyboardButton{}
		for _, v := range items {
			row = append(row, telegram.InlineKeyboardButton{Text: v[0], CallbackData: "shell|" + token + "|" + v[1]})
		}
		rows = append(rows, row)
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// shellRichHTML 只把已编码短令牌放进 data 属性，输出内容始终先 HTML 转义。
func shellRichHTML(body string, kb *telegram.InlineKeyboardMarkup) string {
	var out strings.Builder
	out.WriteString(body)
	if kb != nil {
		for _, row := range kb.InlineKeyboard {
			out.WriteString("<tg-button-row>")
			for _, button := range row {
				out.WriteString(`<tg-button type="callback_data" data="` + escapeHTML(button.CallbackData) + `">` + escapeHTML(button.Text) + `</tg-button>`)
			}
			out.WriteString("</tg-button-row>")
		}
	}
	return out.String()
}

// escapedTailBudget 保留尽可能多的转录且不截断实体或 UTF-8。
func escapedTailBudget(text string, budget int) string {
	runes := []rune(text)
	start := len(runes)
	used := 0
	for start > 0 {
		n := len(escapeHTML(string(runes[start-1])))
		if used+n > budget {
			break
		}
		used += n
		start--
	}
	return escapeHTML(string(runes[start:]))
}
func shellBody(s *shellSession, plain bool) string {
	output := s.output.text
	if s.showHistory && plain {
		output = strings.Join(s.history, "\n")
	}
	// 首次连接和清屏使用可见占位；历史折叠块尤其不能只有标题和空正文。
	if strings.TrimSpace(output) == "" {
		output = "等待终端输出…"
	}
	budget := 24000
	if plain {
		budget = 3000
	}
	body := fmt.Sprintf("<b>🖥 %s · 持久 TTY</b>\n行式转录（非全屏终端）；发送文本后自动 Enter。\n仅发起者可操作；15分钟无输入自动关闭。\n<pre>%s</pre>", escapeHTML(truncName(s.name, 50)), escapedTailBudget(output, budget))
	if !plain {
		history := strings.Join(s.history, "\n")
		if strings.TrimSpace(history) == "" {
			history = "暂无命令历史"
		}
		body += "<details><summary>最近命令历史</summary><pre>" + escapedTailBudget(history, 3000) + "</pre></details>"
	}
	return body
}
func (b *Bot) renderShell(s *shellSession, initial bool) error {
	s.renderMu.Lock()
	defer s.renderMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	body := shellBody(s, s.plain)
	fallbackBody := shellBody(s, true)
	if !initial && body == s.lastShown {
		s.mu.Unlock()
		return nil
	}
	kb := s.keyboard
	plain := s.plain
	messageID := s.resultMsgID
	s.mu.Unlock()
	rich := shellRichHTML(body, kb)
	var err error
	if messageID > 0 {
		if !plain {
			err = s.client.EditRichMessage(s.chatID, messageID, rich, nil)
		}
		if plain || telegram.CanFallbackRichMessage(err) {
			plain = true
			err = s.client.EditMessageText(s.chatID, messageID, fallbackBody, kb)
		}
	} else {
		if !plain {
			messageID, err = s.client.SendRichMessageReturnID(s.chatID, rich, nil)
		}
		if plain || telegram.CanFallbackRichMessage(err) {
			plain = true
			messageID, err = s.client.SendMessageReturnID(s.chatID, fallbackBody, kb)
		}
	}
	if err == nil {
		s.mu.Lock()
		if plain {
			body = fallbackBody
		}
		s.lastShown = body
		s.resultMsgID = messageID
		s.plain = plain
		s.mu.Unlock()
	}
	return err
}
func (b *Bot) finishShell(chatID int64, s *shellSession) {
	if !s.close() {
		return
	}
	// 与在途编辑串行，关闭状态不能被最后一个输出刷新覆盖。
	go func() {
		s.renderMu.Lock()
		defer s.renderMu.Unlock()
		defer b.removeShell(s)
		s.mu.Lock()
		id := s.resultMsgID
		body := shellBody(s, s.plain) + "\n<b>终端已关闭。</b>"
		fallbackBody := shellBody(s, true) + "\n<b>终端已关闭。</b>"
		kb, _ := b.encodeKeyboard(chatID, b.backToPanelKb(s.id, s.name, s.hostID))
		plain := s.plain
		s.mu.Unlock()
		if id <= 0 {
			return
		}
		if plain {
			_ = s.client.EditMessageText(chatID, id, fallbackBody, kb)
		} else {
			err := s.client.EditRichMessage(chatID, id, shellRichHTML(body, kb), nil)
			if telegram.CanFallbackRichMessage(err) {
				_ = s.client.EditMessageText(chatID, id, fallbackBody, kb)
			}
		}
	}()
}
func (b *Bot) handleShellCallback(chatID int64, cb *telegram.CallbackQuery, messageID int64) bool {
	parts := strings.Split(cb.Data, "|")
	action := ""
	s := b.getShell(chatID)
	if parts[0] == "shell" {
		if len(parts) != 3 || s == nil || !s.authorized(chatID, shellCallbackOwner(cb), messageID, parts[1]) {
			return true
		}
		action = parts[2]
	} else if parts[0] == "shhist" || parts[0] == "shexit" {
		if s == nil || len(parts) < 3 || !s.authorized(chatID, shellCallbackOwner(cb), messageID, s.token) {
			return true
		}
		s.mu.Lock()
		valid := parts[1] == s.id && b.hostFromParts(parts, 3) == s.hostID
		s.mu.Unlock()
		if !valid {
			return true
		}
		if parts[0] == "shexit" {
			action = "close"
		} else {
			action = "history"
		}
	} else {
		return false
	}
	switch action {
	case "close":
		b.finishShell(chatID, s)
	case "history", "output", "clear":
		s.mu.Lock()
		s.lastActive = time.Now()
		s.showHistory = action == "history"
		if action == "clear" {
			s.output.text = ""
		}
		s.mu.Unlock()
	default:
		controls := map[string]string{"esc": "\x1b", "interrupt": "\x03", "tab": "\t", "enter": "\r", "up": "\x1b[A", "down": "\x1b[B", "left": "\x1b[D", "right": "\x1b[C"}
		if data, ok := controls[action]; ok {
			b.enqueueShell(s, shellInput{data: data})
		}
	}
	return true
}

// renewShellKeyboard 延长活跃会话固定按钮的有效期，不在每次刷新生成新令牌。
func (b *Bot) renewShellKeyboard(s *shellSession) {
	s.mu.Lock()
	kb := s.keyboard
	closed := s.closed
	s.mu.Unlock()
	if closed || kb == nil {
		return
	}
	b.callbackMu.Lock()
	defer b.callbackMu.Unlock()
	for _, row := range kb.InlineKeyboard {
		for _, button := range row {
			if record, ok := b.callbackStore[button.CallbackData]; ok {
				record.ExpiresAt = time.Now().Add(callbackTTL)
				b.callbackStore[button.CallbackData] = record
			}
		}
	}
}
