package bot

import (
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"io"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestTerminalOutputSplitUTF8AndEscape(t *testing.T) {
	var o terminalOutput
	input := []byte("你\x1b[31m好\x1b[0m\x1b]0;secret\x1b\\<tag>\n")
	for _, b := range input {
		o.append([]byte{b})
	}
	if o.text != "你好<tag>\n" {
		t.Fatalf("过滤结果 %q", o.text)
	}
	o.append([]byte(strings.Repeat("中", shellOutputBudget)))
	if len(o.text) > shellOutputBudget || !utf8.ValidString(o.text) {
		t.Fatal("输出预算或UTF8损坏")
	}
	if got := escapedTailBudget("<<<<你好", 10); len(got) > 10 || !utf8.ValidString(got) {
		t.Fatal(got)
	}
}
func TestShellOwnerAndNonce(t *testing.T) {
	s := &shellSession{chatID: 1, ownerID: 2, resultMsgID: 3, token: "nonce"}
	if !s.authorized(1, 2, 3, "nonce") {
		t.Fatal("拒绝所有者")
	}
	for _, v := range [][3]int64{{1, 4, 3}, {9, 2, 3}, {1, 2, 8}, {1, 0, 3}} {
		if s.authorized(v[0], v[1], v[2], "nonce") {
			t.Fatal("越权")
		}
	}
	if s.authorized(1, 2, 3, "old") {
		t.Fatal("旧会话nonce有效")
	}
	for _, m := range []*telegram.Message{
		{From: &telegram.User{ID: 2}, SenderChat: &telegram.Chat{ID: 1}},
		{From: &telegram.User{ID: 2}, ForwardDate: 1},
		{From: &telegram.User{ID: 2}, ForwardOrigin: []byte(`{"type":"user"}`)},
		{From: &telegram.User{ID: 2, IsBot: true}},
	} {
		if shellMessageOwner(m) != 0 {
			t.Fatal("接受匿名或转发")
		}
	}
	if shellMessageOwner(&telegram.Message{From: &telegram.User{ID: 2}, ForwardOrigin: []byte("null")}) != 2 {
		t.Fatal("null不应视为转发")
	}
}

type mockShell struct {
	mu      sync.Mutex
	closed  int
	writes  []string
	signal  chan struct{}
	written chan struct{}
}

func (m *mockShell) Read(p []byte) (int, error) { <-m.signal; return 0, io.EOF }
func (m *mockShell) Write(p []byte) (int, error) {
	m.mu.Lock()
	m.writes = append(m.writes, string(p))
	m.mu.Unlock()
	if m.written != nil {
		m.written <- struct{}{}
	}
	return len(p), nil
}
func (m *mockShell) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed++
	if m.closed == 1 {
		close(m.signal)
	}
	return nil
}
func TestShellCloseAndConcurrentInput(t *testing.T) {
	term := &mockShell{signal: make(chan struct{})}
	s := &shellSession{chatID: 1, ownerID: 2, done: make(chan struct{}), input: make(chan shellInput, 32), terminal: term}
	b := &Bot{shells: map[int64]*shellSession{1: s}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.enqueueShell(s, shellInput{data: " text \n"}); s.close() }()
	}
	wg.Wait()
	if term.closed != 1 {
		t.Fatalf("Close次数 %d", term.closed)
	}
	if b.enqueueShell(s, shellInput{data: "bad"}) {
		t.Fatal("关闭后输入")
	}
	b.stopShells()
	if b.getShell(1) != nil {
		t.Fatal("Stop会话泄露")
	}
}
func TestShellWritePreservesTextAndHistory(t *testing.T) {
	term := &mockShell{signal: make(chan struct{}), written: make(chan struct{}, 32)}
	s := &shellSession{done: make(chan struct{}), input: make(chan shellInput, 32), terminal: term}
	b := &Bot{}
	ended := make(chan struct{})
	go func() { b.shellWriteLoop(s, term); close(ended) }()
	for i := 0; i < 25; i++ {
		if !b.enqueueShell(s, shellInput{data: "  echo hi  \n", history: true}) {
			t.Fatal("队列异常")
		}
	}
	for i := 0; i < 25; i++ {
		<-term.written
	}
	s.close()
	<-ended
	if len(s.history) != 20 {
		t.Fatalf("历史条数 %d", len(s.history))
	}
	if term.writes[0] != "  echo hi  \n" {
		t.Fatal("空白被修改")
	}
}
func TestShellRichEncodedButtons(t *testing.T) {
	b := &Bot{}
	kb, err := b.encodeKeyboard(1, shellKeyboard("nonce"))
	if err != nil {
		t.Fatal(err)
	}
	html := shellRichHTML("<pre>&lt;script&gt;</pre>", kb)
	if strings.Contains(html, "shell|nonce") || !strings.Contains(html, `data="cb1:`) {
		t.Fatal("未使用短令牌")
	}
	s := &shellSession{name: "<name>", output: terminalOutput{text: strings.Repeat("<中>", 20000)}}
	if body := shellBody(s, false); !strings.Contains(body, "<details>") || strings.Contains(body, "<中>") {
		t.Fatal("Rich过滤或折叠历史缺失")
	}
	if body := shellBody(s, true); len(body) > 4096 || !utf8.ValidString(body) {
		t.Fatal("普通消息预算失效")
	}
}

func TestShellIgnoresNonText(t *testing.T) {
	b := &Bot{}
	s := &shellSession{input: make(chan shellInput, 2)}
	b.runShellCommand(1, s, "", 2)
	if len(s.input) != 0 {
		t.Fatal("非文本消息变成Enter")
	}
	b.runShellCommand(1, s, " ", 3)
	if in := <-s.input; in.data != " \n" {
		t.Fatal("空格未保留")
	}
}
