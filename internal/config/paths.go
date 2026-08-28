package config

// 图标 / 静态资源相关的持久化路径集中定义，作为全局单一事实来源（Single Source of Truth）。
// 说明：历史代码里同一目录曾出现相对路径("data/images")与绝对路径("/data/images")混用，
// 导致"上传写到 /app/data/images、静态服务读 /data/images"两边对不上。
// 这里统一用绝对路径，所有 handler 一律引用本文件常量，杜绝再次分叉。
const (
	// DataDir 持久化根目录（Docker VOLUME 挂载点）。
	DataDir = "/data"

	// ImagesDir 图标图片统一存放目录：手动上传与自动抓取的 favicon 都落到这里，
	// 对应静态路由 /images/<file>。
	ImagesDir = "/data/images"

	// ConfigDir 配置目录。
	ConfigDir = "/data/config"

	// IconsConfigPath 图标绑定配置文件（IconItem 数组格式）。
	IconsConfigPath = "/data/config/icons.json"

	// LegacyImageDir 历史图标目录，仅为兼容旧数据保留静态访问，新逻辑不再写入。
	LegacyImageDir = "/data/config/image"
)
