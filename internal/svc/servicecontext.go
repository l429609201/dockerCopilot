package svc

import (
	"github.com/docker/docker/client"
	"github.com/l429609201/dockerCopilot/internal/config"
	"github.com/l429609201/dockerCopilot/internal/module"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"sort"
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
	// DockerClient 本地默认 client，等价于 DockerManager.Local()。
	// 保留此字段是为兼容大量既有代码；跨主机操作请用 DockerManager.GetClient(hostID)。
	DockerClient *client.Client
	// DockerManager 多 Docker 主机连接池，按 hostID 路由到不同 client。
	DockerManager *DockerManager
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
	// 镜像详细信息（用于更新完成消息）
	OldImageDigest string `json:"oldImageDigest,omitempty"` // 旧镜像 SHA256
	NewImageDigest string `json:"newImageDigest,omitempty"` // 新镜像 SHA256
	OldImageSize   int64  `json:"oldImageSize,omitempty"`   // 旧镜像大小（字节）
	NewImageSize   int64  `json:"newImageSize,omitempty"`   // 新镜像大小（字节）
	ImageName      string `json:"imageName,omitempty"`      // 镜像名称（不含tag）
	ImageTag       string `json:"imageTag,omitempty"`       // 镜像标签
	// Layers 镜像拉取时各分层(layer)的实时进度，供前端任务中心展开显示。
	// 仅拉取类任务有值，其它任务为空（omitempty 向后兼容）。
	Layers []LayerProgress `json:"layers,omitempty"`
}

// LayerProgress 单个镜像分层(layer)的拉取进度。
type LayerProgress struct {
	ID         string `json:"id"`               // 层短ID，如 e42181a960c8
	Status     string `json:"status"`           // 状态：Downloading/Extracting/Pull complete/Already exists 等
	Current    int64  `json:"current,omitempty"` // 已处理字节
	Total      int64  `json:"total,omitempty"`   // 总字节
	Percentage int    `json:"percentage"`       // 该层百分比 0-100
}

type ProgressStoreType map[string]TaskProgress

func NewServiceContext(c config.Config) *ServiceContext {
	svcCtx := &ServiceContext{
		Config:        c,
		HubImageInfo:  module.NewImageCheck(),
		ProgressStore: make(ProgressStoreType),
	}
	// 默认并发上限取配置值，未配置时回退为 2，避免同时拉取过多镜像压垮 Docker daemon
	maxConcurrent := c.Task.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	svcCtx.TaskManager = NewTaskManager(maxConcurrent)
	// 加载持久化配置（定时更新、Registry凭据、Bot、Docker主机），失败时使用默认配置
	svcCtx.AppConfig = appconfig.NewStore()
	// 保证本地 Docker 主机始终存在（首项、不可删）
	svcCtx.AppConfig.EnsureLocalHost()
	// 按主机列表构建多 Docker 连接池；DockerClient 指向本地默认 client 以兼容既有代码
	svcCtx.DockerManager = NewDockerManager(svcCtx.AppConfig.ListDockerHosts())
	svcCtx.DockerClient = svcCtx.DockerManager.Local()
	if svcCtx.DockerClient == nil {
		logx.Error("本地 Docker client 初始化失败，请检查 docker.sock 是否挂载")
	}
	return svcCtx
}

// ReloadDockerHosts 按最新配置重建 Docker 连接池，并刷新本地默认 client 引用。
// 供多 Docker 主机配置变更后调用。
func (ctx *ServiceContext) ReloadDockerHosts() {
	ctx.AppConfig.EnsureLocalHost()
	ctx.DockerManager.Reload(ctx.AppConfig.ListDockerHosts())
	ctx.DockerClient = ctx.DockerManager.Local()
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

// maxDoneKept 已完成任务在内存中保留的最大条数，超出则清理最旧的，避免无限增长。
const maxDoneKept = 50

// ListProgress 返回全部任务快照，按开始时间倒序（最新在前）。
// 同时清理超量的已完成任务：运行中任务全部保留，已完成任务仅保留最近 maxDoneKept 条。
func (ctx *ServiceContext) ListProgress() []TaskProgress {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	all := make([]TaskProgress, 0, len(ctx.ProgressStore))
	for _, p := range ctx.ProgressStore {
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].StartedAt > all[j].StartedAt
	})

	doneCount := 0
	for _, p := range all {
		if p.IsDone {
			doneCount++
			if doneCount > maxDoneKept {
				delete(ctx.ProgressStore, p.TaskID)
			}
		}
	}
	if doneCount <= maxDoneKept {
		return all
	}
	kept := make([]TaskProgress, 0, len(ctx.ProgressStore))
	for _, p := range all {
		if _, ok := ctx.ProgressStore[p.TaskID]; ok {
			kept = append(kept, p)
		}
	}
	return kept
}
