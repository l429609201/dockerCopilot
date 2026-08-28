package compose

import (
	"context"
	"os"

	"github.com/google/uuid"
	composeMod "github.com/l429609201/dockerCopilot/internal/module/compose"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
)

// Action 对 Compose 项目执行部署类操作（up/down/restart/pull 等）。
// 操作放入统一任务系统异步执行，返回 taskID 供前端轮询进度。
// 对高风险配置需 confirmWarnings=true 才放行（除非配置允许高风险）。
func (l *ComposeLogic) Action(req *types.ComposeActionReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if !composeMod.IsSupportedAction(req.Action) {
		return bad(resp, "不支持的操作: "+req.Action), nil
	}
	dir, decErr := composeMod.DecodeID(req.ID)
	if decErr != nil {
		return bad(resp, "非法项目ID"), nil
	}
	resolvedDir, _, sErr := composeMod.SafeResolveDir(l.scanPaths(), dir)
	if sErr != nil {
		return bad(resp, sErr.Error()), nil
	}
	scanner := composeMod.NewScanner([]string{resolvedDir}, 1)
	projects := scanner.Scan()
	if len(projects) == 0 {
		return bad(resp, "该目录下未找到 compose 文件"), nil
	}
	composeFile := projects[0].ComposeFile

	// up 操作前做风险检查：读取主文件校验，存在高风险且未确认且未全局允许时拦截
	if req.Action == "up" && !l.allowHighRisk() && !req.ConfirmWarnings {
		filePath, _ := composeMod.SafeResolveFile(resolvedDir, composeFile)
		if content, readErr := os.ReadFile(filePath); readErr == nil {
			vr := composeMod.Validate(content)
			if vr.HasWarnings() {
				resp.Code = 409
				resp.Msg = "存在高风险配置，请确认后再部署"
				resp.Data = map[string]interface{}{"warnings": vr.Warnings, "needConfirm": true}
				return resp, nil
			}
		}
	}

	taskID := uuid.New().String()
	action := req.Action
	timeoutSec := l.commandTimeoutSec()
	projectName := projects[0].Name
	// 以项目ID作为资源键，避免同一项目并发部署
	startErr := l.svcCtx.TaskManager.TryStart(taskID, "compose:"+req.ID, svc.TaskTypeComposeAction, func(taskCtx context.Context) {
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID: taskID, Name: "Compose " + action + " " + projectName,
			Percentage: 10, Message: "正在执行 " + action, DetailMsg: "正在执行 docker compose " + action,
			TaskType: svc.TaskTypeComposeAction, ResourceID: req.ID,
		})
		// 使用带进度更新的版本，实时推送日志到前端
		result := composeMod.RunActionWithProgress(taskCtx, resolvedDir, composeFile, action, timeoutSec, l.svcCtx, taskID)
		progress := svc.TaskProgress{
			TaskID: taskID, Name: "Compose " + action + " " + projectName,
			Percentage: 100, IsDone: true, TaskType: svc.TaskTypeComposeAction, ResourceID: req.ID,
			DetailMsg: truncateOutput(result.Output),
		}
		if result.Success {
			progress.Message = "执行成功"
		} else {
			progress.Message = "执行失败"
			progress.Failed = true
		}
		l.svcCtx.UpdateProgress(taskID, progress)
	})
	if startErr != nil {
		return bad(resp, startErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}

// truncateOutput 限制回显输出长度，避免进度体积过大。
func truncateOutput(s string) string {
	const max = 4000
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "...(输出已截断)"
	}
	return s
}
