package containerops

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

const terminalOpenTimeout = 30 * time.Second

// Terminal 是单个持久 Docker TTY；Read 为合并的原始终端流，不使用 stdcopy 解帧。
// 调用方负责用户归属、闲置回收及流渲染；一个读取者可与多个写入者并发使用。
// OpenTerminal 的 ctx 只控制建立过程，成功后必须显式 Close 释放连接。
type Terminal interface {
	io.ReadWriteCloser
	Resize(ctx context.Context, cols, rows uint) error
}

// terminalClient 将真实 Docker SDK 与隔离测试分开，测试不连接 Docker socket。
type terminalClient interface {
	ContainerInspect(context.Context, string) (types.ContainerJSON, error)
	ContainerExecCreate(context.Context, string, container.ExecOptions) (types.IDResponse, error)
	ContainerExecAttach(context.Context, string, container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecResize(context.Context, string, container.ResizeOptions) error
}

type dockerTerminal struct {
	client    terminalClient
	execID    string
	attach    types.HijackedResponse
	cancel    context.CancelFunc
	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    atomic.Bool
}

// OpenTerminal 仅对当前 Service 的主机及完整容器 ID 建立交互终端，不按名查找或重试。
// shell 支持空值/sh//bin/sh 或 bash//bin/bash；缺少所选 shell 时直接报错。
func (s *Service) OpenTerminal(ctx context.Context, containerID, shell string) (Terminal, error) {
	cli, err := s.cliOrErr()
	if err != nil {
		return nil, err
	}
	return openTerminal(ctx, cli, containerID, shell)
}

func openTerminal(ctx context.Context, cli terminalClient, containerID, shell string) (Terminal, error) {
	if len(containerID) != 64 {
		return nil, fmt.Errorf("终端目标必须使用完整容器 ID")
	}
	if _, err := hex.DecodeString(containerID); err != nil {
		return nil, fmt.Errorf("终端目标容器 ID 无效")
	}
	switch shell {
	case "", "sh", "/bin/sh":
		shell = "/bin/sh"
	case "bash", "/bin/bash":
		shell = "/bin/bash"
	default:
		return nil, fmt.Errorf("终端仅支持 sh 或 bash")
	}
	buildCtx, buildCancel := context.WithTimeout(ctx, terminalOpenTimeout)
	defer buildCancel()
	inspected, err := cli.ContainerInspect(buildCtx, containerID)
	if err != nil {
		return nil, fmt.Errorf("核验终端容器失败: %w", err)
	}
	if inspected.ContainerJSONBase == nil || inspected.ID != containerID {
		return nil, fmt.Errorf("终端容器身份不一致")
	}
	if inspected.State == nil || !inspected.State.Running || inspected.State.Paused {
		return nil, fmt.Errorf("终端容器必须处于运行且未暂停状态")
	}
	created, err := cli.ContainerExecCreate(buildCtx, containerID, container.ExecOptions{
		AttachStdin: true, AttachStdout: true, AttachStderr: true, Tty: true,
		Cmd: []string{shell, "-i"}, Env: []string{"TERM=xterm"},
	})
	if err != nil {
		return nil, fmt.Errorf("创建终端失败: %w", err)
	}
	if created.ID == "" {
		return nil, fmt.Errorf("创建终端未返回 exec ID")
	}
	// 将建立阶段的取消转发给 attach，但成功后解除转发；不能 defer cancel 活连接。
	liveCtx, liveCancel := context.WithCancel(context.WithoutCancel(ctx))
	stopForward := context.AfterFunc(buildCtx, liveCancel)
	attached, err := cli.ContainerExecAttach(liveCtx, created.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		stopForward()
		liveCancel()
		return nil, fmt.Errorf("连接终端失败（不会自动重试）: %w", err)
	}
	t := &dockerTerminal{client: cli, execID: created.ID, attach: attached, cancel: liveCancel}
	if attached.Conn == nil || attached.Reader == nil {
		stopForward()
		_ = t.Close()
		return nil, fmt.Errorf("终端 attach 返回无效连接")
	}
	// 在返回前确认 resize 可用，固定默认列行顺序为 80 列、24 行。
	err = t.Resize(buildCtx, 80, 24)
	stopped := stopForward()
	if err == nil {
		err = buildCtx.Err()
	}
	if err != nil || !stopped {
		_ = t.Close()
		if err == nil {
			err = context.Canceled
		}
		return nil, fmt.Errorf("初始化终端失败: %w", err)
	}
	return t, nil
}

func (t *dockerTerminal) Read(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return t.attach.Reader.Read(p)
}

func (t *dockerTerminal) Write(p []byte) (int, error) {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	// 不记录输入；互斥保证不同消息写入不会在连接中交错。
	return t.attach.Conn.Write(p)
}

// Resize 的参数始终为列、行，拒绝零值及超过 PTY 16 位窗口范围的尺寸。
func (t *dockerTerminal) Resize(ctx context.Context, cols, rows uint) error {
	if cols == 0 || rows == 0 || cols > 65535 || rows > 65535 {
		return fmt.Errorf("终端列数和行数必须在 1..65535 之间")
	}
	if t.closed.Load() {
		return io.ErrClosedPipe
	}
	resizeCtx, cancel := context.WithTimeout(ctx, terminalOpenTimeout)
	defer cancel()
	return t.client.ContainerExecResize(resizeCtx, t.execID, container.ResizeOptions{Width: cols, Height: rows})
}

// Close 幂等释放 attach，不关闭 Service 共享的 Docker client。
// 无并发写入时短暂尝试 Ctrl-C、清行、exit；忙碌时直接断开以免被输入阻塞。
// Docker 没有 kill exec API，断开 TTY/发送 exit 均不能强保证 shell/子进程退出；
// 这里绝不为清理 shell 而停止或杀死整个容器。
func (t *dockerTerminal) Close() error {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		if t.attach.Conn != nil {
			if t.writeMu.TryLock() {
				// 仅在底层支持写 deadline 时尝试退出，防止 Close 无限等待。
				if err := t.attach.Conn.SetWriteDeadline(time.Now().Add(150 * time.Millisecond)); err == nil {
					_, _ = t.attach.Conn.Write([]byte("\x03\x15exit\n"))
				}
				t.writeMu.Unlock()
			}
			t.attach.Close()
		}
		t.cancel()
	})
	return nil
}
