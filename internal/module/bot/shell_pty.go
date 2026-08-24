package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// ptyShell 管理一个基于 Docker API exec 的交互式 Shell 会话。
// 通过 ContainerExecAttach(Tty=true) 直连容器，无需宿主机安装 docker CLI。
type ptyShell struct {
	svcCtx      *svc.ServiceContext
	containerID string
	chatID      int64
	resultMsgID int64

	execID string                      // Docker exec 实例 ID（用于 resize）
	hijack dockertypes.HijackedResponse // 双向流：Reader 读输出，Conn 写输入
	mutex  sync.Mutex

	buffer      bytes.Buffer // 累积输出缓冲
	lastUpdate  time.Time    // 上次更新消息的时间
	outputLines []string     // 输出行缓存（用于增量更新）

	ctx    context.Context
	cancel context.CancelFunc
}

// ANSI/终端控制序列过滤正则集合。
// 之前仅用 \x1b\[[0-9;]*[a-zA-Z] 无法匹配含私有参数前缀(?、>、=)的 CSI 序列，
// 导致 bracketed-paste 开关 \x1b[?2004h / \x1b[?2004l 等残留在输出里（显示为 [?2004h）。
var (
	// CSI 序列：ESC [ 后跟可选私有前缀(?<>=)、参数字节(0-9;:)、中间字节(空格~/)，以最终字节(@~)结束
	ansiCSI = regexp.MustCompile("\x1b\\[[?>=]?[0-9;:]*[ -/]*[@-~]")
	// OSC 序列：ESC ] ... 以 BEL(\a) 或 ESC \ (ST) 结束，用于设置窗口标题等
	ansiOSC = regexp.MustCompile("\x1b\\][^\a\x1b]*(?:\a|\x1b\\\\)")
	// 其它单字符 ESC 序列（如 ESC ( B 之类的字符集切换的残留 ESC）
	ansiOther = regexp.MustCompile("\x1b[@-Z\\\\-_]")
	// 孤立的控制字符（保留 \n \r \t，其余不可见控制符去除，避免终端脏字符）
	ctrlChars = regexp.MustCompile("[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
)

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

// Start 启动 Shell 会话：通过 Docker API 在目标容器内创建 TTY exec 并挂载双向流。
// 不依赖宿主机的 docker CLI，避免容器内 $PATH 无 docker 导致启动失败。
func (p *ptyShell) Start(shell string) error {
	execResp, err := p.svcCtx.DockerClient.ContainerExecCreate(p.ctx, p.containerID, container.ExecOptions{
		Cmd:          []string{shell},
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("创建 exec 失败: %w", err)
	}
	p.execID = execResp.ID

	hijack, err := p.svcCtx.DockerClient.ContainerExecAttach(p.ctx, execResp.ID, container.ExecStartOptions{
		Tty: true,
	})
	if err != nil {
		return fmt.Errorf("挂载 exec 流失败: %w", err)
	}
	p.hijack = hijack

	// 设置终端尺寸（80列 x 24行，标准终端尺寸），失败不影响会话可用性
	if err := p.svcCtx.DockerClient.ContainerExecResize(p.ctx, execResp.ID, container.ResizeOptions{
		Height: 24,
		Width:  80,
	}); err != nil {
		logx.Errorf("设置 exec 终端尺寸失败: %v", err)
	}

	return nil
}

// Write 向容器 Shell 写入数据（发送命令）
func (p *ptyShell) Write(data []byte) (int, error) {
	if p.hijack.Conn == nil {
		return 0, fmt.Errorf("Shell 会话未初始化")
	}
	return p.hijack.Conn.Write(data)
}

// ReadOutput 读取 PTY 输出并累积到缓冲区
// 返回自上次读取后的新增内容
func (p *ptyShell) ReadOutput(timeout time.Duration) (string, error) {
	if p.hijack.Reader == nil {
		return "", fmt.Errorf("Shell 会话未初始化")
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
				n, err := p.hijack.Reader.Read(buf)
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

// stripANSI 移除 ANSI/终端控制序列，按 CSI→OSC→其它 ESC→孤立控制符的顺序清理，
// 保证 Telegram 里展示的终端输出干净（无 [?2004h 之类脏字符）。
func (p *ptyShell) stripANSI(s string) string {
	s = ansiCSI.ReplaceAllString(s, "")
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiOther.ReplaceAllString(s, "")
	s = ctrlChars.ReplaceAllString(s, "")
	return s
}

// Close 关闭 Shell 会话并清理资源。
// exec 进程随连接关闭由 Docker 守护进程回收，无需额外 kill。
func (p *ptyShell) Close() error {
	p.cancel() // 取消上下文，停止读取 goroutine

	if p.hijack.Conn != nil {
		p.hijack.Close()
	}
	return nil
}
