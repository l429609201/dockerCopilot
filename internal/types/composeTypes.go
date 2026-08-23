package types

// 以下为 Compose 项目管理相关的请求类型，手写维护。

// ComposeFileReq 读取指定项目的指定文件。
type ComposeFileReq struct {
	ID       string `path:"id"`
	Filename string `path:"filename"`
}

// ComposeFileSaveReq 保存指定项目的指定文件内容。
type ComposeFileSaveReq struct {
	ID       string `path:"id"`
	Filename string `path:"filename"`
	Content  string `json:"content"`
}

// ComposeValidateReq 校验 compose 内容。
type ComposeValidateReq struct {
	Content string `json:"content"`
}

// ComposeActionReq 对项目执行部署类操作。
// ConfirmWarnings 为 true 时表示用户已确认高风险配置。
type ComposeActionReq struct {
	ID              string `path:"id"`
	Action          string `json:"action"`
	ConfirmWarnings bool   `json:"confirmWarnings,optional"`
}

// ComposeIDReq 按项目ID操作。
type ComposeIDReq struct {
	ID string `path:"id"`
}

// ComposeConfigReq 保存 Compose 扫描配置（前端提交，写入持久化 AppConfig）。
// 所有字段可选：ScanPaths 为项目扫描根目录列表；其余为扫描/执行参数。
type ComposeConfigReq struct {
	ScanPaths         []string `json:"scanPaths,optional"`
	MaxDepth          int      `json:"maxDepth,optional"`
	MaxFileSize       int64    `json:"maxFileSize,optional"`
	CommandTimeoutSec int      `json:"commandTimeoutSec,optional"`
	AllowHighRisk     bool     `json:"allowHighRisk,optional"`
}
