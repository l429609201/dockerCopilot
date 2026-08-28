package compose

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/l429609201/dockerCopilot/internal/svc"
)

// ActionResult 承载一次 compose 命令的执行结果。
type ActionResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	Duration int64  `json:"durationMs"`
}

// supportedActions 支持的 compose 子命令白名单，防止任意命令注入。
var supportedActions = map[string][]string{
	"up":      {"up", "-d"},
	"down":    {"down"},
	"restart": {"restart"},
	"pull":    {"pull"},
	"stop":    {"stop"},
	"start":   {"start"},
}

// IsSupportedAction 判断动作是否受支持。
func IsSupportedAction(action string) bool {
	_, ok := supportedActions[action]
	return ok
}

// RunAction 在指定项目目录执行 docker compose 子命令。
// 通过白名单限定子命令，使用 context 控制超时，合并 stdout/stderr 输出。
// 如果提供了 svcCtx 和 taskID，会实时更新任务进度（流式输出日志）。
func RunAction(ctx context.Context, projectDir, composeFile, action string, timeoutSec int) ActionResult {
	return RunActionWithProgress(ctx, projectDir, composeFile, action, timeoutSec, nil, "")
}

// RunActionWithProgress 执行 compose 命令并实时更新任务进度。
// svcCtx 和 taskID 用于实时推送日志到前端；为 nil 时降级为 RunAction 行为。
func RunActionWithProgress(ctx context.Context, projectDir, composeFile, action string, timeoutSec int, svcCtx *svc.ServiceContext, taskID string) ActionResult {
	start := time.Now()
	args, ok := supportedActions[action]
	if !ok {
		return ActionResult{Success: false, Output: "不支持的操作: " + action}
	}
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// 组装 docker compose -f <file> <action...>
	fullArgs := []string{"compose", "-f", composeFile}
	fullArgs = append(fullArgs, args...)
	cmd := exec.CommandContext(cmdCtx, "docker", fullArgs...)
	cmd.Dir = projectDir

	// 创建管道捕获 stdout 和 stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ActionResult{Success: false, Output: fmt.Sprintf("创建输出管道失败: %s", err.Error()), Duration: time.Since(start).Milliseconds()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ActionResult{Success: false, Output: fmt.Sprintf("创建错误管道失败: %s", err.Error()), Duration: time.Since(start).Milliseconds()}
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		// 特殊处理：docker 命令未找到的情况，给出更清晰的提示
		if strings.Contains(err.Error(), "executable file not found") {
			return ActionResult{
				Success: false,
				Output: fmt.Sprintf("❌ Docker 命令未找到\n\n"+
					"可能的原因：\n"+
					"1. Docker 未安装或未加入 PATH 环境变量\n"+
					"2. 容器内运行需要：\n"+
					"   - 挂载 Docker socket: -v /var/run/docker.sock:/var/run/docker.sock\n"+
					"   - 容器内安装 Docker CLI\n\n"+
					"原始错误：%s", err.Error()),
				Duration: time.Since(start).Milliseconds(),
			}
		}
		return ActionResult{Success: false, Output: fmt.Sprintf("启动命令失败: %s", err.Error()), Duration: time.Since(start).Milliseconds()}
	}

	// 实时读取并更新进度
	var outputBuf bytes.Buffer
	outputChan := make(chan string, 100)
	done := make(chan struct{})

	// 合并 stdout 和 stderr 的输出
	go streamOutput(stdout, outputChan)
	go streamOutput(stderr, outputChan)

	// 实时更新任务进度
	go func() {
		for line := range outputChan {
			outputBuf.WriteString(line)
			outputBuf.WriteString("\n")

			// 如果提供了 svcCtx 和 taskID，实时更新进度
			if svcCtx != nil && taskID != "" {
				progress, exists := svcCtx.GetProgress(taskID)
				if exists {
					// 保留完整输出，限制长度避免过大
					fullOutput := outputBuf.String()
					if len(fullOutput) > 8000 {
						// 只保留最后 8000 字符
						fullOutput = "...(前面内容已截断)\n" + fullOutput[len(fullOutput)-8000:]
					}
					progress.DetailMsg = fullOutput
					svcCtx.UpdateProgress(taskID, progress)
				}
			}
		}
		close(done)
	}()

	// 等待命令完成
	err = cmd.Wait()
	close(outputChan)
	<-done

	duration := time.Since(start).Milliseconds()
	output := strings.TrimSpace(outputBuf.String())

	if cmdCtx.Err() == context.DeadlineExceeded {
		return ActionResult{Success: false, Output: fmt.Sprintf("命令超时(%ds)\n%s", timeoutSec, output), Duration: duration}
	}
	if err != nil {
		return ActionResult{Success: false, Output: fmt.Sprintf("执行失败: %s\n%s", err.Error(), output), Duration: duration}
	}
	return ActionResult{Success: true, Output: output, Duration: duration}
}

// streamOutput 从 reader 读取输出并发送到 channel
func streamOutput(reader io.Reader, output chan<- string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		output <- scanner.Text()
	}
}
