package appconfig

// AppConfig 是持久化到 /data/config/config.json 的全局配置。
// 集中管理定时更新、Registry 凭据和机器人配置，供调度器、拉取登录和 Bot 复用。
type AppConfig struct {
	// Registries 保存 Registry 登录凭据（如 Docker Hub），按名称索引。
	Registries []RegistryCredential `json:"registries"`
	// ScheduledUpdates 定时更新任务列表。
	ScheduledUpdates []ScheduledUpdateRule `json:"scheduledUpdates"`
	// ScheduledUpdateCron 全局定时更新 cron 表达式（五段式：分 时 日 月 周）。
	// 所有启用的规则共用这一个时间，到点统一依次执行；各规则自身的 Cron 字段已废弃。
	ScheduledUpdateCron string `json:"scheduledUpdateCron"`
	// Telegram 机器人配置。
	Telegram TelegramConfig `json:"telegram"`
}

// RegistryCredential 单个 Registry 的登录凭据。
// Password 为敏感字段，对外输出时必须脱敏，禁止回显明文。
type RegistryCredential struct {
	ID       string `json:"id"`       // 唯一标识
	Name     string `json:"name"`     // 展示名称
	Registry string `json:"registry"` // registry 地址，空表示 Docker Hub
	Username string `json:"username"` // 用户名
	Password string `json:"password"` // 密码或访问令牌（敏感）
}

// 定时任务类型常量。老数据无 Type 字段时按 update 处理，保证向后兼容。
const (
	RuleTypeUpdate = "update" // 自动更新容器
	RuleTypePrune  = "prune"  // 自动清理镜像
	RuleTypeBackup = "backup" // 自动备份容器配置
)

// 镜像清理范围常量（仅 prune 类型使用）。
const (
	PruneModeDangling = "dangling" // 仅清理无 tag 的悬空镜像
	PruneModeUnused   = "unused"   // 清理所有未被容器使用的镜像
)

// ScheduledUpdateRule 定时任务规则（历史名称保留，现已支持更新/清理/备份多种类型）。
type ScheduledUpdateRule struct {
	ID   string `json:"id"`   // 唯一标识
	Name string `json:"name"` // 规则名称
	// Type 任务类型：update(自动更新) / prune(自动清理镜像) / backup(自动备份)。
	// 为空时按 update 处理，兼容历史数据。
	Type string `json:"type,omitempty"`
	// Enabled 是否启用。
	Enabled bool `json:"enabled"`
	// Cron 已废弃：调度改为使用全局 AppConfig.ScheduledUpdateCron，此字段仅为向后兼容保留。
	Cron string `json:"cron,omitempty"`
	// PruneMode 镜像清理范围（仅 prune 类型使用）：dangling(无tag) / unused(未使用)。
	PruneMode string `json:"pruneMode,omitempty"`
	// ContainerNames 需要纳入本规则的容器名列表。
	ContainerNames []string `json:"containerNames"`
	// OnlyWhenUpdate 仅在检测到有新版本时才执行更新。
	OnlyWhenUpdate bool `json:"onlyWhenUpdate"`
	// SkipInvalidTag 跳过无 tag 或 digest 形式（sha256:）的镜像，避免误更新。
	SkipInvalidTag bool `json:"skipInvalidTag"`
	// RegistryID 拉取时使用的凭据ID，空表示匿名拉取。
	RegistryID string `json:"registryId"`
	// KeepOldContainer 更新后是否保留旧容器（不删除）。
	KeepOldContainer bool `json:"keepOldContainer"`
	// NotifyOnStart / NotifyOnDone / NotifyOnError 通知开关。
	NotifyOnStart bool `json:"notifyOnStart"`
	NotifyOnDone  bool `json:"notifyOnDone"`
	NotifyOnError bool `json:"notifyOnError"`
	// LastRunAt / LastResult 记录最近一次执行的时间与结果摘要。
	LastRunAt  int64  `json:"lastRunAt,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
}

// TelegramConfig 机器人配置，Token 为敏感字段。
type TelegramConfig struct {
	Enabled bool `json:"enabled"`
	// Token 机器人 Token（敏感）。
	Token string `json:"token"`
	// AllowedChatIDs 允许交互的会话ID白名单，仅这些用户可执行操作。
	AllowedChatIDs []int64 `json:"allowedChatIds"`
	// Proxy 可选代理地址（形如 http://host:port 或 socks5://host:port）。
	Proxy string `json:"proxy"`
	// PollIntervalSec 长轮询间隔(秒)，默认由调用方兜底。
	PollIntervalSec int `json:"pollIntervalSec"`
	// 通知总开关及分类开关。
	NotifyUpdate bool `json:"notifyUpdate"`
	// UpdateCheckIntervalMinutes 内置更新检测周期（分钟），<=0 时用默认 30。
	UpdateCheckIntervalMinutes int `json:"updateCheckIntervalMinutes,omitempty"`
	// MutedContainers 已屏蔽"有更新"周期通知的容器名列表，命中则不推送。
	MutedContainers []string `json:"mutedContainers,omitempty"`
	// NotifiedVersions 记录已推送过更新通知的容器→镜像版本，用于去重避免刷屏。
	// key 为容器名，value 为该次通知对应的镜像引用；镜像变化才会重新推送。
	NotifiedVersions map[string]string `json:"notifiedVersions,omitempty"`
}

// defaultConfig 返回带合理默认值的空配置。
func defaultConfig() *AppConfig {
	return &AppConfig{
		Registries:          []RegistryCredential{},
		ScheduledUpdates:    []ScheduledUpdateRule{},
		ScheduledUpdateCron: "30 4 * * *", // 默认每天 04:30
		Telegram: TelegramConfig{
			AllowedChatIDs:  []int64{},
			PollIntervalSec: 3,
		},
	}
}
