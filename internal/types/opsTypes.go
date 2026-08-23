package types

// 阶段7：Portainer 风格容器运维相关请求类型。

// ContainerActionReq 简单容器操作（pause/unpause/kill/start/stop/restart）。
type ContainerActionReq struct {
	Id string `path:"id"`
}

// ContainerRemoveReq 删除容器请求。
type ContainerRemoveReq struct {
	Id            string `path:"id"`
	Force         bool   `form:"force,default=false"`
	RemoveVolumes bool   `form:"removeVolumes,default=false"`
}

// ContainerRenameReq2 重命名容器（避免与已有 goctl 生成类型冲突）。
type ContainerRenameReq2 struct {
	Id      string `path:"id"`
	NewName string `json:"newName"`
}

// ContainerLogsReq 查看容器日志请求。
type ContainerLogsReq struct {
	Id         string `path:"id"`
	Tail       int    `form:"tail,default=200"`
	Timestamps bool   `form:"timestamps,default=false"`
	Since      string `form:"since,optional"`
}

// ContainerExecReq 容器内命令执行请求。
type ContainerExecReq struct {
	Id      string   `path:"id"`
	Cmd     []string `json:"cmd"`
	WorkDir string   `json:"workDir,optional"`
	User    string   `json:"user,optional"`
}

// ContainerInspectReq 查看容器完整配置。
type ContainerInspectReq struct {
	Id string `path:"id"`
}

// ContainerEditReq 容器参数编辑请求。
// 仅包含首期支持安全编辑的字段；未提供的字段保留原容器配置。
// 编辑通过"重建"完成（Docker 不支持这些字段原地修改）。
type ContainerEditReq struct {
	Id string `path:"id"`
	// Image 新镜像（可选，为空保留原镜像）。
	Image string `json:"image,optional"`
	// Env 环境变量列表（形如 KEY=VALUE），非 nil 时整体替换。
	Env []string `json:"env,optional"`
	// RestartPolicy 重启策略（no/always/unless-stopped/on-failure），空表示不改。
	RestartPolicy string `json:"restartPolicy,optional"`
	// PortBindings 端口映射（形如 "8080:80/tcp"），非 nil 时整体替换。
	PortBindings []string `json:"portBindings,optional"`
	// KeepOldContainer 重建后是否保留旧容器（默认删除）。
	KeepOldContainer bool `json:"keepOldContainer,optional"`
	// ConfirmWarnings 是否已确认高风险变更。
	ConfirmWarnings bool `json:"confirmWarnings,optional"`
}


// ===== 容器文件管理相关请求 =====

// FileListReq 列出容器内目录。
type FileListReq struct {
	Id   string `path:"id"`
	Path string `form:"path,default=/"`
}

// FileReadReq 读取文本文件（在线预览/编辑）。
type FileReadReq struct {
	Id   string `path:"id"`
	Path string `form:"path"`
}

// FileDownloadReq 下载文件。
type FileDownloadReq struct {
	Id   string `path:"id"`
	Path string `form:"path"`
}

// FileWriteReq 写入/保存文本文件内容。
type FileWriteReq struct {
	Id      string `path:"id"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FileMkdirReq 新建目录。
type FileMkdirReq struct {
	Id   string `path:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
}

// FileDeleteReq 删除文件/目录。
type FileDeleteReq struct {
	Id   string `path:"id"`
	Path string `json:"path"`
}

// FileRenameReq 重命名/移动。
type FileRenameReq struct {
	Id  string `path:"id"`
	Src string `json:"src"`
	Dst string `json:"dst"`
}