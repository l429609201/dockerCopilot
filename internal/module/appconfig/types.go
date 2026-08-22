package appconfig

// AppConfig 是持久化到 /data/config/config.json 的全局配置。
// 集中管理定时更新、Registry 凭据和机器人配置，供调度器、拉取登录和 Bot 复用。
type AppConfig struct {
	// Registries 保存 Registry 登录凭据（如 Docker Hub），按名称索引。
	Registries []RegistryCredential `json:"registries"`
	// ScheduledUpdates 定时更新任务列表。
	ScheduledUpdates []ScheduledUpdateRule `json:"scheduledUpdates"`
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

// ScheduledUpdateRule 定时更新一批容器的规则。
type ScheduledUpdateRule struct {
	ID   string `json:"id"`   // 唯一标识
	Name string `json:"name"` // 规则名称
	// Enabled 是否启用。
	Enabled bool `json:"enabled"`
	// Cron 五段式 cron 表达式（分 时 日 月 周）。
	Cron string `json:"cron"`
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
}

// defaultConfig 返回带合理默认值的空配置。
func defaultConfig() *AppConfig {
	return &AppConfig{
		Registries:       []RegistryCredential{},
		ScheduledUpdates: []ScheduledUpdateRule{},
		Telegram: TelegramConfig{
			AllowedChatIDs:  []int64{},
			PollIntervalSec: 3,
		},
	}
}
