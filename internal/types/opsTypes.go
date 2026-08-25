package types

// 阶段7：Portainer 风格容器运维相关请求类型。

// ContainerActionReq 简单容器操作（pause/unpause/kill/start/stop/restart）。
// HostID 标记容器所属 Docker 主机（多 Docker 管理），空表示本地。
type ContainerActionReq struct {
	Id     string `path:"id"`
	HostID string `form:"hostId,optional"`
}

// ContainerRemoveReq 删除容器请求。
type ContainerRemoveReq struct {
	Id            string `path:"id"`
	Force         bool   `form:"force,default=false"`
	RemoveVolumes bool   `form:"removeVolumes,default=false"`
	HostID        string `form:"hostId,optional"`
}

// ContainerRenameReq2 重命名容器（避免与已有 goctl 生成类型冲突）。
type ContainerRenameReq2 struct {
	Id      string `path:"id"`
	NewName string `json:"newName"`
	HostID  string `json:"hostId,optional"`
}

// ContainerLogsReq 查看容器日志请求。
type ContainerLogsReq struct {
	Id         string `path:"id"`
	Tail       int    `form:"tail,default=200"`
	Timestamps bool   `form:"timestamps,default=false"`
	Since      string `form:"since,optional"`
	HostID     string `form:"hostId,optional"`
}

// ContainerExecReq 容器内命令执行请求。
type ContainerExecReq struct {
	Id      string   `path:"id"`
	Cmd     []string `json:"cmd"`
	WorkDir string   `json:"workDir,optional"`
	User    string   `json:"user,optional"`
	HostID  string   `json:"hostId,optional"`
}

// ContainerInspectReq 查看容器完整配置。
type ContainerInspectReq struct {
	Id     string `path:"id"`
	HostID string `form:"hostId,optional"`
}

// ContainerEditReq 容器参数编辑请求。
// 未提供的字段保留原容器配置；编辑通过"重建"完成（Docker 不支持这些字段原地修改）。
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
	// Binds 卷/绑定挂载（形如 "/host:/container:ro"），非 nil 时整体替换。
	Binds []string `json:"binds,optional"`
	// NetworkMode 网络模式（bridge/host/none/自定义网络名），空表示不改。
	NetworkMode string `json:"networkMode,optional"`
	// Labels 容器标签键值对，非 nil 时整体替换。
	Labels map[string]string `json:"labels,optional"`
	// Cmd 启动命令，非 nil 时整体覆盖。
	Cmd []string `json:"cmd,optional"`
	// Entrypoint 入口点，非 nil 时整体覆盖。
	Entrypoint []string `json:"entrypoint,optional"`
	// Memory 内存硬限制（字节），指针非 nil 时应用（0=不限制）。
	Memory *int64 `json:"memory,optional"`
	// MemorySwap 内存+swap 限制（字节），指针非 nil 时应用。
	MemorySwap *int64 `json:"memorySwap,optional"`
	// NanoCPUs CPU 限额（cpus×1e9），指针非 nil 时应用（0=不限制）。
	NanoCPUs *int64 `json:"nanoCpus,optional"`
	// ConfirmWarnings 是否已确认高风险变更（如挂载改动）。
	ConfirmWarnings bool `json:"confirmWarnings,optional"`
	// HostID 容器所属 Docker 主机（多 Docker 管理），空表示本地。
	HostID string `json:"hostId,optional"`
}


// ===== 容器文件管理相关请求 =====

// FileListReq 列出容器内目录。
type FileListReq struct {
	Id     string `path:"id"`
	Path   string `form:"path,default=/"`
	HostID string `form:"hostId,optional"`
}

// FileReadReq 读取文本文件（在线预览/编辑）。
type FileReadReq struct {
	Id     string `path:"id"`
	Path   string `form:"path"`
	HostID string `form:"hostId,optional"`
}

// FileDownloadReq 下载文件。
type FileDownloadReq struct {
	Id     string `path:"id"`
	Path   string `form:"path"`
	HostID string `form:"hostId,optional"`
}

// FileWriteReq 写入/保存文本文件内容。
type FileWriteReq struct {
	Id      string `path:"id"`
	Path    string `json:"path"`
	Content string `json:"content"`
	HostID  string `json:"hostId,optional"`
}

// FileMkdirReq 新建目录。
type FileMkdirReq struct {
	Id     string `path:"id"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	HostID string `json:"hostId,optional"`
}

// FileDeleteReq 删除文件/目录。
type FileDeleteReq struct {
	Id     string `path:"id"`
	Path   string `json:"path"`
	HostID string `json:"hostId,optional"`
}

// FileRenameReq 重命名/移动。
type FileRenameReq struct {
	Id     string `path:"id"`
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	HostID string `json:"hostId,optional"`
}