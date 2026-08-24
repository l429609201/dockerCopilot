package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// ptyShell 管理一个基于 PTY 的交互式 Shell 会话
type ptyShell struct {
	svcCtx      *svc.ServiceContext
	containerID string
	chatID      int64
	resultMsgID int64

	cmd   *exec.Cmd
	ptmx  *os.File // PTY master 文件描述符
	mutex sync.Mutex

	buffer      bytes.Buffer // 累积输出缓冲
	lastUpdate  time.Time    // 上次更新消息的时间
	outputLines []string     // 输出行缓存（用于增量更新）

	ctx    context.Context
	cancel context.CancelFunc
}

// ansiStripper 简单的 ANSI 转义序列过滤正则
var ansiStripper = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// newPtyShell 创建一个新的 PTY Shell 会话
func newPtyShell(svcCtx *svc.ServiceContext, containerID string, chatID, resultMsgID int64) *ptyShell {
	ctx, cancel := context.WithCancel(context.Background())
	return &ptyShell{
		svcCtx:      svcCtx,
		containerID: containerID,
		chatID:      chatID,
		resultMsgID: resultMsgID,
		ctx:         ctx,
		cancel:      cancel,
		lastUpdate:  time.Now(),
	}
}

// Start 启动 PTY Shell 会话
func (p *ptyShell) Start(shell string) error {
	// 构建 docker exec 命令，使用 -it 启用 PTY
	p.cmd = exec.CommandContext(p.ctx, "docker", "exec", "-it", p.containerID, shell)

	// 启动带 PTY 的命令
	ptmx, err := pty.Start(p.cmd)
	if err != nil {
		return fmt.Errorf("启动 PTY 失败: %w", err)
	}
	p.ptmx = ptmx

	// 设置终端尺寸（80列 x 24行，标准终端尺寸）
	if err := pty.Setsize(p.ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	}); err != nil {
		logx.Errorf("设置 PTY 尺寸失败: %v", err)
	}

	return nil
}

// Write 向 PTY 写入数据（发送命令）
func (p *ptyShell) Write(data []byte) (int, error) {
	if p.ptmx == nil {
		return 0, fmt.Errorf("PTY 未初始化")
	}
	return p.ptmx.Write(data)
}

// ReadOutput 读取 PTY 输出并累积到缓冲区
// 返回自上次读取后的新增内容
func (p *ptyShell) ReadOutput(timeout time.Duration) (string, error) {
	if p.ptmx == nil {
		return "", fmt.Errorf("PTY 未初始化")
	}

	// 使用带超时的读取
	readCtx, readCancel := context.WithTimeout(p.ctx, timeout)
	defer readCancel()

	var newData bytes.Buffer
	buf := make([]byte, 4096)

	// 启动读取 goroutine
	errChan := make(chan error, 1)
	go func() {
		for {
			select {
			case <-readCtx.Done():
				return
			default:
				n, err := p.ptmx.Read(buf)
				if n > 0 {
					p.mutex.Lock()
					p.buffer.Write(buf[:n])
					newData.Write(buf[:n])
					p.mutex.Unlock()
				}
				if err != nil {
					if err != io.EOF {
						errChan <- err
					}
					return
				}
			}
		}
	}()

	// 等待超时或读取完成
	select {
	case err := <-errChan:
		return p.stripANSI(newData.String()), err
	case <-readCtx.Done():
		return p.stripANSI(newData.String()), nil
	}
}

// GetFullOutput 获取完整的累积输出（用于最终展示）
func (p *ptyShell) GetFullOutput() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.stripANSI(p.buffer.String())
}

// stripANSI 移除 ANSI 转义序列
func (p *ptyShell) stripANSI(s string) string {
	return ansiStripper.ReplaceAllString(s, "")
}

// Close 关闭 PTY 会话并清理资源
func (p *ptyShell) Close() error {
	p.cancel() // 取消上下文

	var errs []error
	if p.ptmx != nil {
		if err := p.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 PTY: %w", err))
		}
	}

	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			errs = append(errs, fmt.Errorf("终止进程: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭错误: %v", errs)
	}
	return nil
}
