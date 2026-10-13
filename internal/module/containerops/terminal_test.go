package containerops

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

const terminalTestID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// 全部测试使用内存连接与 fake SDK，绝不探测本机 Docker socket。
type terminalFakeClient struct {
	inspect                         types.ContainerJSON
	create                          container.ExecOptions
	createTarget                    string
	attachOptions                   container.ExecAttachOptions
	attachCtx                       context.Context
	conn                            *terminalFakeConn
	resize                          []container.ResizeOptions
	creates, attaches               int
	createErr, attachErr, resizeErr error
	attachHook                      func(context.Context)
}

func newTerminalFake() *terminalFakeClient {
	return &terminalFakeClient{
		inspect: types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{
			ID: terminalTestID, State: &types.ContainerState{Running: true},
		}},
		conn: &terminalFakeConn{},
	}
}
func (f *terminalFakeClient) ContainerInspect(context.Context, string) (types.ContainerJSON, error) {
	return f.inspect, nil
}
func (f *terminalFakeClient) ContainerExecCreate(_ context.Context, id string, opts container.ExecOptions) (types.IDResponse, error) {
	f.creates++
	f.createTarget, f.create = id, opts
	return types.IDResponse{ID: "exec-id"}, f.createErr
}
func (f *terminalFakeClient) ContainerExecAttach(ctx context.Context, id string, opts container.ExecAttachOptions) (types.HijackedResponse, error) {
	f.attaches++
	f.attachCtx, f.attachOptions = ctx, opts
	if f.attachHook != nil {
		f.attachHook(ctx)
	}
	if f.attachErr != nil {
		return types.HijackedResponse{}, f.attachErr
	}
	return types.HijackedResponse{Conn: f.conn, Reader: bufio.NewReader(strings.NewReader("tty-output"))}, nil
}
func (f *terminalFakeClient) ContainerExecResize(_ context.Context, id string, opts container.ResizeOptions) error {
	f.resize = append(f.resize, opts)
	return f.resizeErr
}

type terminalFakeConn struct {
	mu         sync.Mutex
	writes     bytes.Buffer
	closed     atomic.Int32
	writing    atomic.Int32
	overlapped atomic.Bool
}

func (c *terminalFakeConn) Read([]byte) (int, error) { return 0, io.EOF }
func (c *terminalFakeConn) Write(p []byte) (int, error) {
	if c.writing.Add(1) != 1 {
		c.overlapped.Store(true)
	}
	defer c.writing.Add(-1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() != 0 {
		return 0, net.ErrClosed
	}
	return c.writes.Write(p)
}
func (c *terminalFakeConn) Close() error                     { c.closed.Add(1); return nil }
func (c *terminalFakeConn) LocalAddr() net.Addr              { return nil }
func (c *terminalFakeConn) RemoteAddr() net.Addr             { return nil }
func (c *terminalFakeConn) SetDeadline(time.Time) error      { return nil }
func (c *terminalFakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *terminalFakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestTerminalCreateAndLifetime(t *testing.T) {
	f := newTerminalFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	term, err := openTerminal(ctx, f, terminalTestID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if f.createTarget != terminalTestID || !f.create.AttachStdin || !f.create.AttachStdout || !f.create.AttachStderr || !f.create.Tty {
		t.Fatalf("unexpected create: %#v", f.create)
	}
	if !reflect.DeepEqual(f.create.Cmd, []string{"/bin/sh", "-i"}) || !f.attachOptions.Tty {
		t.Fatalf("unexpected shell or attach: %#v / %#v", f.create.Cmd, f.attachOptions)
	}
	if len(f.resize) != 1 || f.resize[0].Width != 80 || f.resize[0].Height != 24 {
		t.Fatal(f.resize)
	}
	cancel()
	if f.attachCtx.Err() != nil {
		t.Fatal("caller cancellation closed successful attach")
	}
	out, err := io.ReadAll(term)
	if err != nil || string(out) != "tty-output" {
		t.Fatalf("read %q: %v", out, err)
	}
	if _, err = term.Write([]byte("echo test\n")); err != nil {
		t.Fatal(err)
	}
	if err = term.Resize(context.Background(), 120, 40); err != nil {
		t.Fatal(err)
	}
	if last := f.resize[len(f.resize)-1]; last.Width != 120 || last.Height != 40 {
		t.Fatal(last)
	}
	for i := 0; i < 2; i++ {
		if err := term.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if f.conn.closed.Load() != 1 || f.attachCtx.Err() == nil {
		t.Fatal("close did not release once")
	}
	if !strings.Contains(f.conn.writes.String(), "\x03\x15exit\n") {
		t.Fatal("missing best-effort exit")
	}
	if _, err := term.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if err := term.Resize(context.Background(), 80, 24); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

func TestTerminalValidationAndNoRetry(t *testing.T) {
	for _, shell := range []string{"sh", "/bin/sh", "bash", "/bin/bash"} {
		f := newTerminalFake()
		term, err := openTerminal(context.Background(), f, terminalTestID, shell)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(f.create.Cmd[0], strings.TrimPrefix(shell, "/bin/")) {
			t.Fatal(f.create.Cmd)
		}
		_ = term.Close()
	}
	for _, tc := range []struct {
		name, id, shell  string
		mismatch, paused bool
	}{
		{name: "name", id: "container-name"},
		{name: "short", id: "aaaaaaaaaaaa"},
		{name: "bad hex", id: strings.Repeat("z", 64)},
		{name: "bad shell", id: terminalTestID, shell: "sh -c id"},
		{name: "identity", id: terminalTestID, mismatch: true},
		{name: "paused", id: terminalTestID, paused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTerminalFake()
			if tc.mismatch {
				f.inspect.ID = strings.Repeat("b", 64)
			}
			f.inspect.State.Paused = tc.paused
			if _, err := openTerminal(context.Background(), f, tc.id, tc.shell); err == nil {
				t.Fatal("accepted invalid target")
			}
			if f.creates != 0 {
				t.Fatal("created exec for invalid target")
			}
		})
	}
	for _, stage := range []string{"create", "attach", "resize"} {
		t.Run(stage, func(t *testing.T) {
			f := newTerminalFake()
			sentinel := errors.New("failure")
			switch stage {
			case "create":
				f.createErr = sentinel
			case "attach":
				f.attachErr = sentinel
			case "resize":
				f.resizeErr = sentinel
			}
			if _, err := openTerminal(context.Background(), f, terminalTestID, "bash"); !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if f.creates != 1 || f.attaches > 1 {
				t.Fatal("unexpected retry")
			}
			if stage == "resize" && f.conn.closed.Load() != 1 {
				t.Fatal("leaked attach")
			}
		})
	}
}

func TestTerminalEstablishmentCancel(t *testing.T) {
	f := newTerminalFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.attachHook = func(attachCtx context.Context) {
		cancel()
		select {
		case <-attachCtx.Done():
		case <-time.After(time.Second):
			t.Fatal("attach not cancelled")
		}
	}
	if _, err := openTerminal(ctx, f, terminalTestID, "sh"); err == nil {
		t.Fatal("accepted cancelled attach")
	}
	if f.conn.closed.Load() != 1 {
		t.Fatal("leaked cancelled attach")
	}
}

func TestTerminalResizeValidationAndSerializedWrites(t *testing.T) {
	f := newTerminalFake()
	term, err := openTerminal(context.Background(), f, terminalTestID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	for _, size := range [][2]uint{{0, 24}, {80, 0}, {65536, 24}, {80, 65536}} {
		if err := term.Resize(context.Background(), size[0], size[1]); err == nil {
			t.Fatal(size)
		}
	}
	if len(f.resize) != 1 {
		t.Fatal("invalid size reached client")
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = term.Write([]byte("input\n")) }()
	}
	wg.Wait()
	if f.conn.overlapped.Load() {
		t.Fatal("concurrent writes reached connection")
	}
}

func TestTerminalCloseBoundsExitWrite(t *testing.T) {
	// 对端不读取退出序列，验证 Close 的尽力退出不会永久阻塞。
	left, right := net.Pipe()
	defer right.Close()
	_, cancel := context.WithCancel(context.Background())
	term := &dockerTerminal{attach: types.HijackedResponse{Conn: left, Reader: bufio.NewReader(left)}, cancel: cancel}
	done := make(chan struct{})
	go func() { _ = term.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("best-effort exit write blocked close")
	}
}

func TestTerminalCloseUnblocksBusyWrite(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	ctx, cancel := context.WithCancel(context.Background())
	term := &dockerTerminal{attach: types.HijackedResponse{Conn: left, Reader: bufio.NewReader(left)}, cancel: cancel}
	term.writeMu.Lock()
	done := make(chan struct{})
	go func() { _ = term.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close waited for busy writer")
	}
	term.writeMu.Unlock()
	if ctx.Err() == nil {
		t.Fatal("live context not cancelled")
	}
}
