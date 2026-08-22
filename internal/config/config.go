package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	Auth struct { // JWT 认证需要的密钥和过期时间配置
		AccessSecret string
		AccessExpire int64
	}
	// Task 异步任务相关配置，控制并发与超时。
	Task struct {
		// MaxConcurrent 全局异步任务并发上限，默认 2。
		MaxConcurrent int `json:",default=2"`
		// PullTimeoutSec 单个镜像拉取/容器更新的整体超时(秒)，默认 1800。
		PullTimeoutSec int `json:",default=1800"`
	}
	// Compose 项目管理相关配置。
	Compose struct {
		// ScanPaths Compose 项目扫描根目录列表；空表示禁用 Compose 功能。
		ScanPaths []string `json:",optional"`
		// MaxDepth 扫描时的最大目录深度，默认 3。
		MaxDepth int `json:",default=3"`
		// MaxFileSize 单个 Compose 文件最大字节数，默认 10MB。
		MaxFileSize int64 `json:",default=10485760"`
		// CommandTimeoutSec docker compose 命令执行超时(秒)，默认 300。
		CommandTimeoutSec int `json:",default=300"`
		// AllowHighRisk 是否允许部署包含高风险配置(privileged 等)的项目，默认 false。
		AllowHighRisk bool `json:",default=false"`
	}
}

var (
	Version   string
	BuildDate string
)
