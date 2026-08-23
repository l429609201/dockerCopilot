package types

// 以下为改造新增的请求/响应类型，手写维护，独立于 goctl 生成的 types.go。

// ScheduledRuleReq 定时更新规则的创建/更新请求。
// ID 为空表示新建，非空表示更新指定规则。
type ScheduledRuleReq struct {
	ID               string   `json:"id,optional"`
	Name             string   `json:"name"`
	Type             string   `json:"type,optional"`      // 任务类型：update/prune/backup，空按 update
	PruneMode        string   `json:"pruneMode,optional"` // 清理范围：dangling/unused（仅 prune）
	Enabled          bool     `json:"enabled,optional"`
	Cron             string   `json:"cron,optional"` // 已废弃：调度改用全局 cron，保留仅为兼容
	ContainerNames   []string `json:"containerNames,optional"`
	OnlyWhenUpdate   bool     `json:"onlyWhenUpdate,optional"`
	SkipInvalidTag   bool     `json:"skipInvalidTag,optional"`
	RegistryID       string   `json:"registryId,optional"`
	KeepOldContainer bool     `json:"keepOldContainer,optional"`
	NotifyOnStart    bool     `json:"notifyOnStart,optional"`
	NotifyOnDone     bool     `json:"notifyOnDone,optional"`
	NotifyOnError    bool     `json:"notifyOnError,optional"`
}

// ScheduledRuleIDReq 按 ID 操作规则的请求（删除、立即执行）。
type ScheduledRuleIDReq struct {
	ID string `path:"id"`
}

// CronConfigReq 全局定时更新 cron 配置的更新请求。
type CronConfigReq struct {
	Cron string `json:"cron"`
}

// RegistryReq Registry 凭据的创建/更新请求。
// Password 为空且 ID 非空时表示"保持原密码不变"。
type RegistryReq struct {
	ID       string `json:"id,optional"`
	Type     string `json:"type,optional"` // 类型：dockerhub/github/custom，空视为 custom
	Name     string `json:"name"`
	Registry string `json:"registry,optional"`
	Username string `json:"username"`
	Password string `json:"password,optional"`
	Note     string `json:"note,optional"` // 说明/备注，描述该凭据的用途
}

// RegistryIDReq 按 ID 操作凭据的请求。
type RegistryIDReq struct {
	ID string `path:"id"`
}

// RegistryRateLimitResp Docker Hub 剩余拉取次数的响应数据。
type RegistryRateLimitResp struct {
	Supported bool   `json:"supported"`       // 该凭据是否支持次数查询（仅 Docker Hub）
	Limit     int    `json:"limit"`           // 周期内总配额
	Remaining int    `json:"remaining"`       // 剩余次数
	Source    string `json:"source"`          // anonymous/authenticated
	Message   string `json:"message,omitempty"` // 不支持或出错时的说明
}

// TelegramConfigReq Telegram Bot 配置的更新请求。
// Token 为空表示保持原值不变（配合脱敏回显）。
type TelegramConfigReq struct {
	Enabled         bool    `json:"enabled,optional"`
	Token           string  `json:"token,optional"`
	AllowedChatIDs  []int64 `json:"allowedChatIds,optional"`
	Proxy           string  `json:"proxy,optional"`
	PollIntervalSec            int      `json:"pollIntervalSec,optional"`
	NotifyUpdate               bool     `json:"notifyUpdate,optional"`
	UpdateCheckIntervalMinutes int      `json:"updateCheckIntervalMinutes,optional"` // 内置更新检测周期(分钟)，<=0 用默认 30
	MutedContainers            []string `json:"mutedContainers,optional"`            // 更新检查屏蔽黑名单，命中的容器不推送"有更新"通知
}
