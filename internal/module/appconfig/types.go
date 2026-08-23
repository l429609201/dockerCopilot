package appconfig

import (
	"fmt"
	"strings"
)

// AppConfig 是持久化到 /data/config/config.json 的全局配置。
// 集中管理定时更新、Registry 凭据和机器人配置，供调度器、拉取登录和 Bot 复用。
type AppConfig struct {
	// Registries 保存 Registry 登录凭据（如 Docker Hub），按名称索引。
	Registries []RegistryCredential `json:"registries"`
	// ScheduledUpdates 定时更新任务列表。
	ScheduledUpdates []ScheduledUpdateRule `json:"scheduledUpdates"`
	// Telegram 机器人配置。
	Telegram TelegramConfig `json:"telegram"`
	// Compose 项目管理配置（前端可配置，优先级高于静态 yaml）。
	Compose ComposeConfig `json:"compose"`
}

// ComposeConfig 是 Compose 项目管理的动态配置（持久化到 config.json，前端可编辑）。
// 为空的字段在读取时会回退到静态 yaml 配置，保证向后兼容。
type ComposeConfig struct {
	// ScanPaths Compose 项目扫描根目录列表；空表示未在前端配置（回退静态 yaml）。
	ScanPaths []string `json:"scanPaths,omitempty"`
	// MaxDepth 扫描时的最大目录深度，0 表示回退静态 yaml。
	MaxDepth int `json:"maxDepth,omitempty"`
	// MaxFileSize 单个 Compose 文件最大字节数，0 表示回退静态 yaml。
	MaxFileSize int64 `json:"maxFileSize,omitempty"`
	// CommandTimeoutSec docker compose 命令执行超时(秒)，0 表示回退静态 yaml。
	CommandTimeoutSec int `json:"commandTimeoutSec,omitempty"`
	// AllowHighRisk 是否允许部署包含高风险配置(privileged 等)的项目。
	AllowHighRisk bool `json:"allowHighRisk,omitempty"`
	// Configured 标记用户是否已在前端保存过 Compose 配置。
	// 用于区分"未配置(回退yaml)"与"已配置但清空了某些字段"。
	Configured bool `json:"composeConfigured,omitempty"`
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
	// Cron 该任务的定时表达式（五段式：分 时 日 月 周），支持：
	// 1. 标准 cron 表达式：如 "30 4 * * *"（每天 04:30）
	// 2. 简化配置（自动转换为 cron）：
	//    - "daily:HH:MM" → 每天指定时间，如 "daily:04:30"
	//    - "weekly:N:HH:MM" → 每周指定星期的时间，如 "weekly:1:10:00"（周一10点）
	//    - "hourly:MM" → 每小时指定分钟，如 "hourly:30"（每小时30分）
	//    - "interval:Xh" → 每X小时，如 "interval:6h"（每6小时）
	// 为空或无效时该规则不会被调度。
	Cron string `json:"cron"`
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
		Registries:       []RegistryCredential{},
		ScheduledUpdates: []ScheduledUpdateRule{},
		Telegram: TelegramConfig{
			AllowedChatIDs:  []int64{},
			PollIntervalSec: 3,
		},
	}
}

// ParseCronExpression 将简化配置或标准 cron 表达式转换为标准五段式 cron。
// 支持的简化格式：
//   - "daily:HH:MM" → "MM HH * * *"（每天指定时间）
//   - "weekly:N:HH:MM" → "MM HH * * N"（每周N，0=周日，1=周一...）
//   - "hourly:MM" → "MM * * * *"（每小时指定分钟）
//   - "interval:Xh" → "@every Xh"（robfig/cron 特殊语法）
//   - 标准 cron → 直接返回
// 返回空字符串表示无效配置。
func ParseCronExpression(input string) string {
	if input == "" {
		return ""
	}

	// 已经是标准 cron 表达式（至少包含空格）
	if strings.Contains(input, " ") || strings.HasPrefix(input, "@") {
		return input
	}

	parts := strings.Split(input, ":")
	if len(parts) < 2 {
		return input // 无法识别，直接返回原值
	}

	prefix := parts[0]
	switch prefix {
	case "daily":
		// daily:HH:MM → MM HH * * *
		if len(parts) == 3 {
			hh := parts[1]
			mm := parts[2]
			return fmt.Sprintf("%s %s * * *", mm, hh)
		}
	case "weekly":
		// weekly:N:HH:MM → MM HH * * N
		if len(parts) == 4 {
			weekday := parts[1]
			hh := parts[2]
			mm := parts[3]
			return fmt.Sprintf("%s %s * * %s", mm, hh, weekday)
		}
	case "hourly":
		// hourly:MM → MM * * * *
		if len(parts) == 2 {
			mm := parts[1]
			return fmt.Sprintf("%s * * * *", mm)
		}
	case "interval":
		// interval:Xh → @every Xh
		if len(parts) == 2 {
			duration := parts[1]
			return fmt.Sprintf("@every %s", duration)
		}
	}

	// 无法识别的格式，返回原值
	return input
}
