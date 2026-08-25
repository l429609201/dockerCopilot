package containerops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// ExecResult 承载一次容器内命令执行的结果。
type ExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// Exec 在容器内一次性执行命令，返回合并后的输出与退出码。
// 通过 Docker Exec API 传递参数数组，绝不拼接 shell 字符串，避免命令注入。
// maxOutput 限制返回字节数，timeoutSec 控制整体超时。
func (s *Service) Exec(ctx context.Context, id string, cmd []string, workDir, user string, timeoutSec, maxOutput int) (ExecResult, error) {
	if len(cmd) == 0 {
		return ExecResult{}, fmt.Errorf("命令不能为空")
	}
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	if maxOutput <= 0 {
		maxOutput = 64 * 1024
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cli, err := s.cliOrErr()
	if err != nil {
		return ExecResult{}, err
	}

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   workDir,
		User:         user,
		AttachStdout: true,
		AttachStderr: true,
	}
	created, err := cli.ContainerExecCreate(execCtx, id, execConfig)
	if err != nil {
		return ExecResult{}, fmt.Errorf("创建 exec 失败: %w", err)
	}
	attach, err := cli.ContainerExecAttach(execCtx, created.ID, container.ExecStartOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("附加 exec 失败: %w", err)
	}
	defer attach.Close()

	// 使用 stdcopy 分离并合并 stdout/stderr，限制读取大小防止内存耗尽
	var buf bytes.Buffer
	limited := io.LimitReader(attach.Reader, int64(maxOutput))
	if _, err := stdcopy.StdCopy(&buf, &buf, limited); err != nil && err != io.EOF {
		// 读取错误不致命，返回已获取的部分
		_ = err
	}

	inspect, err := cli.ContainerExecInspect(execCtx, created.ID)
	exitCode := -1
	if err == nil {
		exitCode = inspect.ExitCode
	}
	return ExecResult{Output: buf.String(), ExitCode: exitCode}, nil
}
