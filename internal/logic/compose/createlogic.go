package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	composeMod "github.com/l429609201/dockerCopilot/internal/module/compose"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
)

// Create 从内容创建并部署一个新的 Compose 项目。
// 流程：校验工作目录为绝对路径 → 创建目录 → 校验 YAML → 风险确认 → 写文件 → 任务化 up。
func (l *ComposeLogic) Create(req *types.ComposeCreateReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}

	// 1. 工作目录必须为绝对路径
	workDir := strings.TrimSpace(req.WorkingDir)
	if workDir == "" {
		return bad(resp, "工作目录不能为空"), nil
	}
	if !filepath.IsAbs(workDir) {
		return bad(resp, "工作目录必须为绝对路径"), nil
	}
	workDir = filepath.Clean(workDir)
	// 工作目录为宿主机上的真实路径，不限制在扫描目录内（与 docker compose 原生行为一致）。

	// 2. 文件名：默认 docker-compose.yml，且必须为受支持的 compose 文件名
	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "docker-compose.yml"
	}
	if filename != filepath.Base(filename) || strings.ContainsAny(filename, `/\`) {
		return bad(resp, "非法文件名"), nil
	}
	if !composeMod.IsComposeFileName(filename) {
		return bad(resp, "文件名必须为 docker-compose.yml/yaml 或 compose.yml/yaml"), nil
	}

	// 4. 内容校验
	content := req.Content
	if strings.TrimSpace(content) == "" {
		return bad(resp, "compose 内容不能为空"), nil
	}
	vr := composeMod.Validate([]byte(content))
	if !vr.Valid {
		return bad(resp, "内容校验失败："+vr.Error), nil
	}

	// 5. 高风险配置：未全局允许且未确认时，返回 409 要求确认
	if !l.allowHighRisk() && !req.ConfirmWarnings && vr.HasWarnings() {
		resp.Code = 409
		resp.Msg = "存在高风险配置，请确认后再部署"
		resp.Data = map[string]interface{}{"warnings": vr.Warnings, "needConfirm": true}
		return resp, nil
	}

	// 6. 创建目录（幂等）
	if mkErr := os.MkdirAll(workDir, 0755); mkErr != nil {
		return bad(resp, "创建工作目录失败："+mkErr.Error()), nil
	}

	// 7. 写入 compose 文件（存在则备份）
	filePath := filepath.Join(workDir, filename)
	if old, readErr := os.ReadFile(filePath); readErr == nil {
		_ = os.WriteFile(filePath+".bak", old, 0644)
	}
	if wErr := os.WriteFile(filePath, []byte(content), 0644); wErr != nil {
		return bad(resp, "写入 compose 文件失败："+wErr.Error()), nil
	}

	// 8. 任务化执行 up
	taskID := uuid.New().String()
	projectName := filepath.Base(workDir)
	resourceKey := "compose:" + composeMod.EncodeID(workDir)
	timeoutSec := l.commandTimeoutSec()
	startErr := l.svcCtx.TaskManager.TryStart(taskID, resourceKey, svc.TaskTypeComposeAction, func(taskCtx context.Context) {
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID: taskID, Name: "Compose up " + projectName,
			Percentage: 10, Message: "正在部署", DetailMsg: "正在执行 docker compose up",
			TaskType: svc.TaskTypeComposeAction, ResourceID: resourceKey,
		})
		result := composeMod.RunAction(taskCtx, workDir, filename, "up", timeoutSec)
		progress := svc.TaskProgress{
			TaskID: taskID, Name: "Compose up " + projectName,
			Percentage: 100, IsDone: true, TaskType: svc.TaskTypeComposeAction, ResourceID: resourceKey,
			DetailMsg: truncateOutput(result.Output),
		}
		if result.Success {
			progress.Message = "部署成功"
		} else {
			progress.Message = "部署失败"
			progress.Failed = true
		}
		l.svcCtx.UpdateProgress(taskID, progress)
	})
	if startErr != nil {
		return bad(resp, startErr.Error()), nil
	}

	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"taskID":   taskID,
		"warnings": vr.Warnings,
	}
	return resp, nil
}
