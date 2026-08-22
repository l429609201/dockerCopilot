package compose

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
func RunAction(ctx context.Context, projectDir, composeFile, action string, timeoutSec int) ActionResult {
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
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	duration := time.Since(start).Milliseconds()
	output := strings.TrimSpace(buf.String())
	if cmdCtx.Err() == context.DeadlineExceeded {
		return ActionResult{Success: false, Output: fmt.Sprintf("命令超时(%ds)\n%s", timeoutSec, output), Duration: duration}
	}
	if err != nil {
		return ActionResult{Success: false, Output: fmt.Sprintf("执行失败: %s\n%s", err.Error(), output), Duration: duration}
	}
	return ActionResult{Success: true, Output: output, Duration: duration}
}
