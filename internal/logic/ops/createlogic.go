package ops

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// CreateLogic 处理「从零创建容器」（任务化，支持自动拉取镜像）。
type CreateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLogic {
	return &CreateLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// Create 以任务化方式创建容器，返回 taskID 供前端轮询进度。
// 拉取镜像可能耗时，故放入 TaskManager 异步执行。
func (l *CreateLogic) Create(req *types.CreateContainerReq) (*types.Resp, error) {
	resp := &types.Resp{}
	name := strings.TrimSpace(req.Name)
	image := strings.TrimSpace(req.Image)
	if name == "" || image == "" {
		return fail(resp, "容器名和镜像不能为空"), nil
	}

	spec := containerops.CreateSpec{
		Name:          name,
		Image:         image,
		Env:           req.Env,
		PortBindings:  req.PortBindings,
		Binds:         req.Binds,
		RestartPolicy: req.RestartPolicy,
		NetworkMode:   req.NetworkMode,
		Labels:        req.Labels,
		Cmd:           req.Cmd,
		Entrypoint:    req.Entrypoint,
		AutoPull:      req.AutoPull,
		AutoStart:     req.AutoStart,
	}
	ops := containerops.NewForHost(l.svcCtx, req.HostID)

	taskID := uuid.New().String()
	// 用容器名作为任务资源ID去重，避免同名容器并发创建
	startErr := l.svcCtx.TaskManager.TryStart(taskID, "create:"+name, svc.TaskTypeContainerUpdate, func(taskCtx context.Context) {
		l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
			TaskID: taskID, Name: "创建容器 " + name, Percentage: 5,
			Message: "开始创建", DetailMsg: "开始创建", TaskType: svc.TaskTypeContainerUpdate, ResourceID: name,
		})
		newID, err := ops.Create(taskCtx, spec, func(pct int, msg string) {
			l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
				TaskID: taskID, Name: "创建容器 " + name, Percentage: pct,
				Message: msg, DetailMsg: msg, TaskType: svc.TaskTypeContainerUpdate, ResourceID: name,
			})
		})
		final := svc.TaskProgress{
			TaskID: taskID, Name: "创建容器 " + name, Percentage: 100, IsDone: true,
			TaskType: svc.TaskTypeContainerUpdate, ResourceID: name,
		}
		if err != nil {
			final.Message = "创建失败"
			final.DetailMsg = err.Error()
			final.Failed = true
			logx.Errorf("创建容器 %s 失败: %v", name, err)
		} else {
			final.Message = "创建成功"
			final.DetailMsg = "容器已创建：" + newID
			logx.Infof("审计：创建容器 %s 成功，ID=%s", name, newID)
		}
		l.svcCtx.UpdateProgress(taskID, final)
	})
	if startErr != nil {
		return fail(resp, startErr.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}

// ParseRunCommand 解析 docker run 命令为创建参数（仅解析，不创建）。
// 返回结构与 CreateContainerReq 对齐，供前端回填预览、确认后再走 Create。
func (l *CreateLogic) ParseRunCommand(req *types.ParseRunCommandReq) (*types.Resp, error) {
	resp := &types.Resp{}
	if strings.TrimSpace(req.Command) == "" {
		return fail(resp, "命令不能为空"), nil
	}
	spec, err := containerops.ParseRunCommand(req.Command)
	if err != nil {
		return fail(resp, err.Error()), nil
	}
	resp.Code = 200
	resp.Msg = "success"
	// 字段命名与前端 createContainer 请求体保持一致，便于直接回填
	resp.Data = map[string]interface{}{
		"name":          spec.Name,
		"image":         spec.Image,
		"env":           spec.Env,
		"portBindings":  spec.PortBindings,
		"binds":         spec.Binds,
		"restartPolicy": spec.RestartPolicy,
		"networkMode":   spec.NetworkMode,
		"labels":        spec.Labels,
		"cmd":           spec.Cmd,
		"entrypoint":    spec.Entrypoint,
	}
	return resp, nil
}
