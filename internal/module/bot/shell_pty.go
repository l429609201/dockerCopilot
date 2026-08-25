package bot

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/l429609201/dockerCopilot/internal/svc"
)

// ptyShell 管理一次基于 Docker API exec 的命令执行。
// 采用「一条命令 = 一次非 TTY exec」模型：
//   - 非 TTY 不回显命令、无 shell 提示符、无 bracketed-paste 等 ANSI 控制序列，输出天然干净；
//   - 命令执行结束时输出流自然 EOF，无需哨兵标记或静默超时判定，响应即时；
//   - 退出码通过 ContainerExecInspect 精确获取。
type ptyShell struct {
	svcCtx      *svc.ServiceContext
	containerID string
	hostID      string // 目标容器所属 Docker 主机（多 Docker 管理），空表示本地
	chatID      int64
	resultMsgID int64

	execID string                       // Docker exec 实例 ID（用于查询退出码）
	hijack dockertypes.HijackedResponse // 输出流：Reader 读取多路复用的 stdout/stderr
	mutex  sync.Mutex

	buffer     bytes.Buffer // 累积输出缓冲（已解复用、已清理）
	lastUpdate time.Time    // 上次刷新 Telegram 消息的时间（流式节流用）

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

// newPtyShell 创建一个新的 PTY Shell 会话。hostID 定位容器所属 Docker 主机（空为本地）。
func newPtyShell(svcCtx *svc.ServiceContext, containerID, hostID string, chatID, resultMsgID int64) *ptyShell {
	ctx, cancel := context.WithCancel(context.Background())
	return &ptyShell{
		svcCtx:      svcCtx,
		containerID: containerID,
		hostID:      hostID,
		chatID:      chatID,
		resultMsgID: resultMsgID,
		ctx:         ctx,
		cancel:      cancel,
		lastUpdate:  time.Now(),
	}
}

// cli 返回本会话目标主机的 docker client，未找到回退本地。
func (p *ptyShell) cli() *dockerclient.Client {
	if c, ok := p.svcCtx.DockerManager.GetClient(p.hostID); ok && c != nil {
		return c
	}
	return p.svcCtx.DockerClient
}

// Start 在目标容器内以非 TTY 方式执行给定命令数组，并挂载输出流。
// 不依赖宿主机 docker CLI；非 TTY 保证输出中不含命令回显与终端控制序列。
func (p *ptyShell) Start(cmd []string) error {
	execResp, err := p.cli().ContainerExecCreate(p.ctx, p.containerID, container.ExecOptions{
		Cmd:          cmd,
		Tty:          false,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("创建 exec 失败: %w", err)
	}
	p.execID = execResp.ID

	hijack, err := p.cli().ContainerExecAttach(p.ctx, execResp.ID, container.ExecStartOptions{
		Tty: false,
	})
	if err != nil {
		return fmt.Errorf("挂载 exec 流失败: %w", err)
	}
	p.hijack = hijack

	return nil
}

// ExitCode 查询本次 exec 的退出码。命令仍在运行时返回 -1。
func (p *ptyShell) ExitCode() int {
	if p.execID == "" {
		return -1
	}
	insp, err := p.cli().ContainerExecInspect(context.Background(), p.execID)
	if err != nil {
		return -1
	}
	if insp.Running {
		return -1
	}
	return insp.ExitCode
}

// StartPump 在后台把多路复用的 exec 输出流解复用并持续累积到缓冲区。
// 非 TTY 的 hijack.Reader 是 stdout/stderr 复用流，需用 stdcopy.StdCopy 分离；
// 命令结束时流自然 EOF，StdCopy 返回，done 置位。返回的 channel 关闭即表示读取结束。
func (p *ptyShell) StartPump() <-chan struct{} {
	done := make(chan struct{})
	if p.hijack.Reader == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		// stdout 与 stderr 都写入同一累积缓冲，展示时合并为一段文本
		w := &lockedWriter{sh: p}
		_, _ = stdcopy.StdCopy(w, w, p.hijack.Reader)
	}()
	return done
}

// lockedWriter 将解复用后的输出并发安全地写入 ptyShell.buffer。
type lockedWriter struct{ sh *ptyShell }

func (w *lockedWriter) Write(b []byte) (int, error) {
	w.sh.mutex.Lock()
	defer w.sh.mutex.Unlock()
	return w.sh.buffer.Write(b)
}

// GetFullOutput 获取完整的累积输出（已解复用、已清理 ANSI）。
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

// Close 关闭本次 exec 的输出流并清理资源。
// exec 进程随连接关闭由 Docker 守护进程回收，无需额外 kill。
func (p *ptyShell) Close() error {
	p.cancel() // 取消上下文，停止读取 goroutine

	if p.hijack.Conn != nil {
		p.hijack.Close()
	}
	return nil
}
