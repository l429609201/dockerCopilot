package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
)

func TestShellDirectoryLoadingDoesNotRenderTTY(t *testing.T) {
	s := &shellSession{directoryView: true, directoryLoading: true, output: terminalOutput{text: "terminal-first"}}
	for _, plain := range []bool{true, false} {
		before := shellBody(s, plain)
		s.output.append([]byte("terminal-new\n"))
		after := shellBody(s, plain)
		if before != after || strings.Contains(after, "terminal-") || !strings.Contains(after, "正在读取") {
			t.Fatal("TTY覆盖目录读取态", after)
		}
	}
	s.directoryGeneration = 8
	if s.applyDirectoryResult(7, containerops.TerminalDirectory{Path: "/old"}, "") {
		t.Fatal("旧刷新覆盖新刷新")
	}
}

func TestShellDirectoryPagesAndMetadata(t *testing.T) {
	s := &shellSession{directoryView: true, directory: containerops.TerminalDirectory{Path: "/a\nb<>&", Truncated: true}}
	kinds := []string{"directory", "symlink", "executable", "file", "other"}
	for i := 0; i < 43; i++ {
		s.directory.Entries = append(s.directory.Entries, containerops.TerminalDirectoryEntry{Name: fmt.Sprintf("item%02d.exe", i), Kind: kinds[i%5]})
	}
	body := shellBody(s, true)
	if !strings.Contains(body, "第 1/3 页") || !strings.Contains(body, "已读取 43 项") || !strings.Contains(body, "列表已截断") || strings.Contains(body, "item20") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "📄 <code>\"item03.exe") {
		t.Fatal("按后缀误判或普通文件图标丢失", body)
	}
	s.pageDirectory(-1)
	if s.directoryPage != 0 {
		t.Fatal("首屏越界")
	}
	s.pageDirectory(1)
	s.pageDirectory(1)
	s.pageDirectory(1)
	body = shellBody(s, false)
	if s.directoryPage != 2 || !strings.Contains(body, "item42") || strings.Contains(body, "item39") {
		t.Fatal(body)
	}
}

func TestShellDirectoryEscapingAndBudget(t *testing.T) {
	s := &shellSession{directoryView: true, directory: containerops.TerminalDirectory{Path: strings.Repeat("<\n中", 1000)}}
	for i := 0; i < 20; i++ {
		s.directory.Entries = append(s.directory.Entries, containerops.TerminalDirectoryEntry{Name: strings.Repeat("<\n\r\t\\\"中", 300), Kind: "file"})
	}
	for _, plain := range []bool{true, false} {
		body := shellBody(s, plain)
		if len(body) > 4096 || !utf8.ValidString(body) || strings.Contains(body, "<\n") || !strings.Contains(body, "\\n") || !strings.Contains(body, "名称截断") {
			t.Fatalf("目录转义或预算错误：%d %s", len(body), body)
		}
	}
	if shellDirectoryLabel("a\nb", 100) == shellDirectoryLabel("a\\nb", 100) {
		t.Fatal("换行与字面反斜线混淆")
	}
}

func TestShellDirectoryGenerationCancelAndRawOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &shellSession{directoryView: true, directoryLoading: true, directoryGeneration: 2, directoryCancel: cancel, input: make(chan shellInput, 1), done: make(chan struct{}), output: terminalOutput{text: "unchanged"}}
	b := &Bot{}
	if !b.enqueueShell(s, shellInput{data: "\x03"}) {
		t.Fatal("输入失败")
	}
	if s.directoryView || ctx.Err() == nil {
		t.Fatal("输入未返回终端或取消读取")
	}
	if s.applyDirectoryResult(2, containerops.TerminalDirectory{Path: "/late"}, "") {
		t.Fatal("迟到读取覆盖终端")
	}
	if s.output.text != "unchanged" || (<-s.input).data != "\x03" {
		t.Fatal("TTY内容被改变")
	}
	s.directoryView = true
	generation := s.directoryGeneration
	if !s.applyDirectoryResult(generation, containerops.TerminalDirectory{Path: "/ok"}, "") {
		t.Fatal("有效读取失败")
	}
	s.close()
	if s.applyDirectoryResult(generation, containerops.TerminalDirectory{Path: "/closed"}, "") {
		t.Fatal("关闭后仍接受读取")
	}
}

func TestShellDirectoryCallbacksRequireOwnerNonceMessage(t *testing.T) {
	s := &shellSession{chatID: 1, ownerID: 2, resultMsgID: 3, token: "nonce", directoryView: true, directory: containerops.TerminalDirectory{Entries: make([]containerops.TerminalDirectoryEntry, 30)}}
	b := &Bot{shells: map[int64]*shellSession{1: s}}
	for _, action := range []string{"directory", "dirrefresh", "dirprev", "dirnext", "output"} {
		for _, invalid := range []struct {
			owner, msg int64
			token      string
		}{{4, 3, "nonce"}, {2, 4, "nonce"}, {2, 3, "old"}} {
			cb := &telegram.CallbackQuery{Data: "shell|" + invalid.token + "|" + action, From: &telegram.User{ID: invalid.owner}}
			if !b.handleShellCallback(1, cb, invalid.msg) {
				t.Fatal("未消费目录回调")
			}
			if !s.directoryView || s.directoryPage != 0 || s.directoryLoading {
				t.Fatal("越权更改目录视图")
			}
		}
	}
}
