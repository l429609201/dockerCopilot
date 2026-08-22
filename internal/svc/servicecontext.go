package svc

import (
	"github.com/docker/docker/client"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/module"
	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"sync"
)

type ServiceContext struct {
	Config                     config.Config
	CookieCheckMiddleware      rest.Middleware
	Jwtuuid                    string
	BearerTokenCheckMiddleware rest.Middleware
	JwtSecret                  string
	PortainerJwt               string
	HubImageInfo               *module.ImageUpdateData
	IndexCheckMiddleware       rest.Middleware
	ProgressStore              ProgressStoreType
	DockerClient               *client.Client
	// TaskManager 统一管理异步任务的并发、去重和取消，供容器更新、
	// 定时更新、Compose 操作等复用，避免各处各自 go func 失控。
	TaskManager *TaskManager
	// AppConfig 持久化配置存储（定时更新、Registry凭据、Bot）。
	AppConfig *appconfig.Store
	// Scheduler 定时更新调度器，通过接口解耦避免循环导入（具体实现由 main 注入）。
	Scheduler Reloader
	// Bot Telegram 机器人，通过接口解耦（具体实现由 main 注入）。
	Bot BotController
	mu  sync.Mutex
}

// Reloader 定义调度器对外暴露的最小能力，供 handler 在配置变更后触发重载
// 及按规则ID立即执行。使用接口而非具体类型，避免 svc 反向依赖 scheduler 包造成循环导入。
type Reloader interface {
	Reload()
	// RunNowByID 按规则ID立即异步执行，返回该规则是否存在。
	RunNowByID(ruleID string) bool
}

// BotController 定义 Bot 对外暴露的最小能力，供 handler 在配置变更后触发重载。
type BotController interface {
	Reload()
	Stop()
}

// 任务类型常量，便于前端和通知渠道区分任务来源。
const (
	TaskTypeContainerUpdate = "container_update"
	TaskTypeContainerRestore = "container_restore"
	TaskTypeImagePull       = "image_pull"
	TaskTypeComposeAction   = "compose_action"
	TaskTypeScheduledUpdate = "scheduled_update"
	TaskTypeImagePrune      = "image_prune" // 批量清理/删除镜像
)

type TaskProgress struct {
	TaskID     string `json:"taskID"`
	Percentage int    `json:"percentage"`
	Message    string `json:"message"`
	Name       string `json:"name"`
	DetailMsg  string `json:"detailMsg"`
	IsDone     bool   `json:"isDone"`
	// 以下为增强字段，均可选，保持对旧代码的向后兼容。
	TaskType   string `json:"taskType,omitempty"`   // 任务类型
	ResourceID string `json:"resourceID,omitempty"` // 关联资源（如容器ID）
	Failed     bool   `json:"failed,omitempty"`     // 是否失败结束
	Canceled   bool   `json:"canceled,omitempty"`   // 是否被取消
	StartedAt  int64  `json:"startedAt,omitempty"`  // 开始时间(毫秒)
	EndedAt    int64  `json:"endedAt,omitempty"`    // 结束时间(毫秒)
}

type ProgressStoreType map[string]TaskProgress

func NewServiceContext(c config.Config) *ServiceContext {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logx.Errorf("Unable to create docker client: %s", err)
	}
	svcCtx := &ServiceContext{
		Config:        c,
		HubImageInfo:  module.NewImageCheck(),
		ProgressStore: make(ProgressStoreType),
		DockerClient:  cli,
	}
	// 默认并发上限取配置值，未配置时回退为 2，避免同时拉取过多镜像压垮 Docker daemon
	maxConcurrent := c.Task.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	svcCtx.TaskManager = NewTaskManager(maxConcurrent)
	// 加载持久化配置（定时更新、Registry凭据、Bot），失败时使用默认配置
	svcCtx.AppConfig = appconfig.NewStore()
	return svcCtx
}

func (ctx *ServiceContext) UpdateProgress(taskID string, progress TaskProgress) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	// 首次写入时补充开始时间；结束时补充结束时间，便于前端展示耗时
	existing, ok := ctx.ProgressStore[taskID]
	if !ok || existing.StartedAt == 0 {
		if progress.StartedAt == 0 {
			progress.StartedAt = nowMilli()
		}
	} else if progress.StartedAt == 0 {
		progress.StartedAt = existing.StartedAt
	}
	if progress.IsDone && progress.EndedAt == 0 {
		progress.EndedAt = nowMilli()
	}
	ctx.ProgressStore[taskID] = progress
}

func (ctx *ServiceContext) GetProgress(taskID string) (TaskProgress, bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	progress, ok := ctx.ProgressStore[taskID]
	return progress, ok
}
