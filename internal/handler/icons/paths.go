package icons

import (
	"sync"

	"github.com/l429609201/dockerCopilot/internal/config"
)

// 图标包内路径统一引用全局常量（internal/config/paths.go），避免相对/绝对路径分叉。
var (
	imageUploadDir  = config.ImagesDir       // 图片统一落盘目录（上传+抓取）
	iconsConfigPath = config.IconsConfigPath // 图标配置文件（IconItem 数组格式）
)

// iconsFileMu 保护 icons.json 的读改写（read-modify-write）。
// 批量自动抓取时前端会并发 POST /api/icons/fetch，多个请求同时
// 「读旧 json → 追加自己 → 写回」会互相覆盖，导致大部分写入丢失。
// 所有对 icons.json 的读写都必须在此锁保护下进行。
var iconsFileMu sync.Mutex
