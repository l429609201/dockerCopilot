package containerops

import (
	"bufio"
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// LogsStreamOptions 流式日志读取参数。
type LogsStreamOptions struct {
	Tail       int    // 初始返回最后多少行（<=0 取默认 200，上限 5000）
	Since      string // 起始时间（RFC3339 或 Unix 秒，空表示不限制）
	Timestamps bool   // 是否包含时间戳
	Follow     bool   // 是否持续跟随新日志（类似 docker logs -f）
	Search     string // 后端关键词过滤（不区分大小写），空表示不过滤
}

// LogsStream 流式读取容器日志：逐行读取并通过 onLine 回调实时下发，
// 相比一次性读取，首行可秒级到达，且 Follow 模式下能持续跟随新日志。
//   - onLine 返回 false 时提前终止（用于消费方主动中断，如客户端断连）
//   - 后端在此直接做关键词过滤（等效 docker logs | grep），只把命中行回调出去，
//     从而支持扫描远超前端承载量的日志，且减少传输量。
func (s *Service) LogsStream(ctx context.Context, id string, opts LogsStreamOptions, onLine func(line string) bool) error {
	tail := opts.Tail
	if tail <= 0 {
		tail = 200
	}
	// tail 上限保护：日志文件很大时 daemon 需从末尾回扫定位，N 越大越慢。
	if tail > 5000 {
		tail = 5000
	}

	dockerOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: opts.Timestamps,
		Tail:       itoa(tail),
		Since:      opts.Since,
		Follow:     opts.Follow,
	}
	cli, err := s.cliOrErr()
	if err != nil {
		return err
	}
	// 先探测容器是否为 tty 模式：tty 的日志是原始流（非多路复用），
	// 用 stdcopy 解复用会失败读不到内容，需直接逐行读原始流。
	tty := false
	if insp, ierr := cli.ContainerInspect(ctx, id); ierr == nil && insp.Config != nil {
		tty = insp.Config.Tty
	}

	reader, err := cli.ContainerLogs(ctx, id, dockerOpts)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 关键词过滤（大小写不敏感），空关键词直接放行。
	keyword := strings.ToLower(strings.TrimSpace(opts.Search))
	match := func(line string) bool {
		if keyword == "" {
			return true
		}
		return strings.Contains(strings.ToLower(line), keyword)
	}

	// ctx 取消时关闭 reader，让阻塞的读取（尤其 Follow 模式）立即返回。
	go func() {
		<-ctx.Done()
		_ = reader.Close()
	}()

	// 逐行扫描的数据源：tty 模式直接读原始流；否则用管道 + stdcopy 解复用。
	var lineSrc io.Reader
	if tty {
		lineSrc = reader
	} else {
		pr, pw := io.Pipe()
		go func() {
			_, cerr := stdcopy.StdCopy(pw, pw, reader)
			// 用 CloseWithError 把解复用错误传给读端 Scanner，读端据此结束。
			_ = pw.CloseWithError(cerr)
		}()
		lineSrc = pr
	}

	scanner := bufio.NewScanner(lineSrc)
	// 放大单行缓冲上限，避免超长行（如大 JSON 日志）触发 bufio.ErrTooLong。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		// 消费方要求中断（客户端断连等）
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if !match(line) {
			continue
		}
		if !onLine(line) {
			return nil
		}
	}
	// Scanner 结束：返回扫描错误（io.Pipe 的 CloseWithError 会把解复用错误在此透出；
	// 正常 EOF 时 scanner.Err() 为 nil）。
	if serr := scanner.Err(); serr != nil && serr != io.EOF {
		return serr
	}
	return nil
}
