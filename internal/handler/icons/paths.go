package icons

import "github.com/l429609201/dockerCopilot/internal/config"

// 图标包内路径统一引用全局常量（internal/config/paths.go），避免相对/绝对路径分叉。
var (
	imageUploadDir  = config.ImagesDir     // 图片统一落盘目录（上传+抓取）
	iconsConfigPath = config.IconsConfigPath // 图标配置文件（IconItem 数组格式）
)
