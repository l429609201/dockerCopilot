package containerops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	directoryTimeout    = 10 * time.Second
	directoryMaxOut     = 256 * 1024
	directoryMaxEntries = 1000
)

// TerminalDirectory 是活动交互 shell 的只读目录快照；Truncated 表示条目超过上限。
type TerminalDirectory struct {
	Path      string                   `json:"path"`
	Entries   []TerminalDirectoryEntry `json:"entries"`
	Truncated bool                     `json:"truncated"`
}

type TerminalDirectoryEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Directory 独立执行只读探针，不向活动 TTY 写入字节，也不推测历史 cd。
func (t *dockerTerminal) Directory(ctx context.Context) (TerminalDirectory, error) {
	if t.closed.Load() {
		return TerminalDirectory{}, io.ErrClosedPipe
	}
	probeCtx, cancel := context.WithTimeout(ctx, directoryTimeout)
	defer cancel()
	created, err := t.client.ContainerExecCreate(probeCtx, t.containerID, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", directoryProbe, "directory-probe", t.token},
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return TerminalDirectory{}, fmt.Errorf("创建目录探针失败: %w", err)
	}
	if created.ID == "" {
		return TerminalDirectory{}, fmt.Errorf("目录探针没有 exec ID")
	}
	attached, err := t.client.ContainerExecAttach(probeCtx, created.ID, container.ExecStartOptions{Tty: false})
	if err != nil {
		return TerminalDirectory{}, fmt.Errorf("连接目录探针失败: %w", err)
	}
	defer attached.Close()
	// 单独关闭连接以解阻塞 Reader；仅取消 SDK context 不保证阻塞读能退出。
	stopClose := context.AfterFunc(probeCtx, func() { attached.Close() })
	defer stopClose()
	if attached.Reader == nil || attached.Conn == nil {
		return TerminalDirectory{}, fmt.Errorf("目录探针连接无效")
	}
	var stdout, stderr bytes.Buffer
	limit := &directoryOutputLimit{left: directoryMaxOut}
	_, readErr := stdcopy.StdCopy(limit.writer(&stdout), limit.writer(&stderr), attached.Reader)
	if err := probeCtx.Err(); err != nil {
		return TerminalDirectory{}, fmt.Errorf("目录探针超时或取消: %w", err)
	}
	if readErr != nil {
		return TerminalDirectory{}, fmt.Errorf("读取目录探针失败: %w", readErr)
	}
	inspected, err := t.client.ContainerExecInspect(probeCtx, created.ID)
	if err != nil {
		return TerminalDirectory{}, fmt.Errorf("检查目录探针失败: %w", err)
	}
	if inspected.Running || inspected.ExitCode != 0 {
		// 错误文本仅来自固定脚本，不回显原始文件名或环境变量。
		return TerminalDirectory{}, fmt.Errorf("目录探针不可用（退出码 %d；需 /proc、tr、grep 与 sh）", inspected.ExitCode)
	}
	if t.closed.Load() {
		return TerminalDirectory{}, io.ErrClosedPipe
	}
	return parseTerminalDirectory(stdout.Bytes())
}

type directoryOutputLimit struct{ left int }

type directoryLimitedWriter struct {
	limit *directoryOutputLimit
	out   *bytes.Buffer
}

func (l *directoryOutputLimit) writer(out *bytes.Buffer) io.Writer {
	return directoryLimitedWriter{limit: l, out: out}
}

func (w directoryLimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit.left {
		return 0, errors.New("目录探针输出超过 256KiB")
	}
	w.limit.left -= len(data)
	return w.out.Write(data)
}

// 固定脚本只读 /proc 环境标记和 cwd；shell PID 必须等于 /proc 编号，排除继承标记的子进程。
// cd -P 解析 /proc/PID/cwd 后按物理 PWD 返回，换行路径和文件名由 NUL 字段保留。
// 使用 shell glob，包含隐藏文件；不存在 glob 则由 test 排除。不依赖 ls/find/stat。
const directoryProbe = `set -eu
command -v tr >/dev/null && command -v grep >/dev/null || { echo missing-tools >&2; exit 72; }
found=
for p in /proc/[0-9]*; do
  [ -r "$p/environ" ] || continue
  env=$(tr '\000' '\n' < "$p/environ") || continue
  printf '%s\n' "$env" | grep -Fx "DC_TERMINAL_TOKEN=$1" >/dev/null || continue
  pid=${p##*/}
  printf '%s\n' "$env" | grep -Fx "DC_TERMINAL_PID=$pid" >/dev/null || continue
  found=$pid
  break
done
[ -n "$found" ] || { echo shell-not-found >&2; exit 70; }
cd -P "/proc/$found/cwd" || { echo cwd-unavailable >&2; exit 71; }
[ -r . ] && [ -x . ] || { echo cwd-inaccessible >&2; exit 73; }
printf 'P\000%s\000' "$PWD"
count=0
truncated=0
for f in ./* ./.[!.]* ./..?*; do
  [ -e "$f" ] || [ -L "$f" ] || continue
  if [ "$count" -ge 1000 ]; then truncated=1; break; fi
  n=${f#./}
  if [ -L "$f" ]; then kind=symlink
  elif [ -d "$f" ]; then kind=directory
  elif [ -f "$f" ] && [ -x "$f" ]; then kind=executable
  elif [ -f "$f" ]; then kind=file
  else kind=other
  fi
  printf 'E\000%s\000%s\000' "$kind" "$n"
  count=$((count + 1))
done
printf 'T\000%s\000' "$truncated"`

// 不解析 ls 的文本格式；所有记录必须终止，不接受缺字段或尾部残片。
func parseTerminalDirectory(data []byte) (TerminalDirectory, error) {
	fields := bytes.Split(data, []byte{0})
	if len(fields) < 5 || len(fields[len(fields)-1]) != 0 || string(fields[0]) != "P" || len(fields[1]) == 0 || fields[1][0] != '/' {
		return TerminalDirectory{}, fmt.Errorf("目录探针协议无效")
	}
	result := TerminalDirectory{Path: string(fields[1]), Entries: make([]TerminalDirectoryEntry, 0)}
	i := 2
	for i < len(fields)-1 && string(fields[i]) == "E" {
		if i+2 >= len(fields)-1 || len(result.Entries) >= directoryMaxEntries {
			return TerminalDirectory{}, fmt.Errorf("目录探针条目超限或不完整")
		}
		kind, name := string(fields[i+1]), string(fields[i+2])
		switch kind {
		case "directory", "symlink", "executable", "file", "other":
		default:
			return TerminalDirectory{}, fmt.Errorf("目录探针类型无效")
		}
		if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
			return TerminalDirectory{}, fmt.Errorf("目录探针名称无效")
		}
		result.Entries = append(result.Entries, TerminalDirectoryEntry{Name: name, Kind: kind})
		i += 3
	}
	if i+2 != len(fields)-1 || string(fields[i]) != "T" {
		return TerminalDirectory{}, fmt.Errorf("目录探针结尾无效")
	}
	switch string(fields[i+1]) {
	case "0":
	case "1":
		result.Truncated = true
	default:
		return TerminalDirectory{}, fmt.Errorf("目录探针截断标记无效")
	}
	return result, nil
}
