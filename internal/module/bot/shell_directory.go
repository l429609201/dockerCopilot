package bot

import (
	"context"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
)

// 可选能力保持旧终端实现兼容；目录操作从不向 TTY 写入命令。
type shellDirectoryReader interface {
	Directory(context.Context) (containerops.TerminalDirectory, error)
}

func (b *Bot) readShellDirectory(s *shellSession) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.leaveDirectoryLocked()
	s.directoryView = true
	s.directoryLoading = true
	s.directoryError = ""
	s.directoryPage = 0
	s.lastActive = time.Now()
	generation := s.directoryGeneration
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s.directoryCancel = cancel
	reader, supported := s.terminal.(shellDirectoryReader)
	s.mu.Unlock()
	// 先发布读取态；慢 Docker 请求不占用回调轮询，也不会让输出覆盖目录。
	go func() { _ = b.renderShell(s, false) }()
	go func() {
		defer cancel()
		var directory containerops.TerminalDirectory
		message := ""
		if !supported {
			message = "此终端暂不支持独立目录读取。"
		} else {
			result, err := reader.Directory(ctx)
			if err != nil {
				message = "无法读取当前 Shell 目录，请稍后刷新。"
			} else if result.Path == "" {
				message = "未能确认当前 Shell 目录，请稍后刷新。"
			} else {
				directory = result
			}
			if ctx.Err() != nil {
				message = "目录读取已超时或取消，请重试。"
			}
		}
		if !s.applyDirectoryResult(generation, directory, message) {
			return
		}
		_ = b.renderShell(s, false)
	}()
}

// 持锁确认代次，返回终端、刷新及关闭后的迟到结果一律丢弃。
func (s *shellSession) applyDirectoryResult(generation uint64, directory containerops.TerminalDirectory, message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.directoryView || generation != s.directoryGeneration {
		return false
	}
	s.directory = directory
	s.directoryError = message
	s.directoryLoading = false
	s.directoryCancel = nil
	return true
}

func (s *shellSession) pageDirectory(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.directoryView || s.directoryLoading {
		return
	}
	s.lastActive = time.Now()
	page := s.directoryPage + delta
	if page >= 0 && page*shellDirectoryPageSize < len(s.directory.Entries) {
		s.directoryPage = page
	}
}
