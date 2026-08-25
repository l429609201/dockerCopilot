package containerops

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// Logs 读取容器日志（非流式），受 tail 和最大字节数限制，防止一次读取耗尽内存。
//   - tail：返回最后多少行（<=0 时取默认 200）
//   - since：起始时间（RFC3339 或 Unix 秒，空表示不限制）
//   - timestamps：是否包含时间戳
//   - maxOutput：返回内容字节上限
func (s *Service) Logs(ctx context.Context, id string, tail int, since string, timestamps bool, maxOutput int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	if maxOutput <= 0 {
		maxOutput = 512 * 1024
	}
	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tailStr := itoa(tail)
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: timestamps,
		Tail:       tailStr,
		Since:      since,
	}
	cli, err := s.cliOrErr()
	if err != nil {
		return "", err
	}
	reader, err := cli.ContainerLogs(logCtx, id, options)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// 容器日志为多路复用流，用 stdcopy 解复用为可读文本，并限制读取大小
	var buf bytes.Buffer
	limited := io.LimitReader(reader, int64(maxOutput))
	if _, err := stdcopy.StdCopy(&buf, &buf, limited); err != nil && err != io.EOF {
		// 部分容器（tty 模式）非多路复用流，回退为直接读取
		var raw bytes.Buffer
		lr := io.LimitReader(reader, int64(maxOutput))
		_, _ = io.Copy(&raw, lr)
		if raw.Len() > 0 {
			return raw.String(), nil
		}
	}
	return buf.String(), nil
}

// itoa 简单整数转字符串，避免引入 strconv 到多个文件。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
