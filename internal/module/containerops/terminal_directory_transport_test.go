package containerops

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// 内存 Docker 传输只用于探针测试，不连接任何真实容器。
type directoryTransportFake struct {
	*terminalFakeClient
	response types.HijackedResponse
}

func (f *directoryTransportFake) ContainerExecAttach(context.Context, string, container.ExecAttachOptions) (types.HijackedResponse, error) {
	return f.response, nil
}

func TestDirectoryTransportDoesNotWriteTTY(t *testing.T) {
	var wire bytes.Buffer
	_, _ = stdcopy.NewStdWriter(&wire, stdcopy.Stdout).Write([]byte("P\x00/work\x00E\x00directory\x00data\x00T\x000\x00"))
	f := &directoryTransportFake{terminalFakeClient: newTerminalFake(), response: types.HijackedResponse{Conn: &terminalFakeConn{}, Reader: bufio.NewReader(&wire)}}
	tty := &terminalFakeConn{}
	term := &dockerTerminal{client: f, containerID: terminalTestID, token: "test-token", attach: types.HijackedResponse{Conn: tty}}
	got, err := term.Directory(context.Background())
	if err != nil || got.Path != "/work" || len(got.Entries) != 1 {
		t.Fatalf("目录读取失败: %+v %v", got, err)
	}
	if tty.writes.Len() != 0 || f.create.AttachStdin || f.create.Tty || f.createTarget != terminalTestID {
		t.Fatal("目录操作影响活动TTY或错误目标")
	}
}

func TestDirectoryTransportCancellation(t *testing.T) {
	reader, writer := net.Pipe()
	defer writer.Close()
	f := &directoryTransportFake{terminalFakeClient: newTerminalFake(), response: types.HijackedResponse{Conn: reader, Reader: bufio.NewReader(reader)}}
	term := &dockerTerminal{client: f, containerID: terminalTestID, token: "test-token"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := term.Directory(ctx); finished <- err }()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("取消后未报错")
		}
	case <-time.After(time.Second):
		reader.Close()
		t.Fatal("取消未解除探针阻塞")
	}
}

func TestDirectoryTransportOutputLimit(t *testing.T) {
	var wire bytes.Buffer
	_, _ = stdcopy.NewStdWriter(&wire, stdcopy.Stdout).Write([]byte(strings.Repeat("x", directoryMaxOut+1)))
	f := &directoryTransportFake{terminalFakeClient: newTerminalFake(), response: types.HijackedResponse{Conn: &terminalFakeConn{}, Reader: bufio.NewReader(&wire)}}
	term := &dockerTerminal{client: f, containerID: terminalTestID, token: "test-token"}
	if _, err := term.Directory(context.Background()); err == nil {
		t.Fatal("超限输出未拒绝")
	}
}
