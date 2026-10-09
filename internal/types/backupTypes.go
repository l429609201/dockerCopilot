package types

// BackupCreateReq 创建实质备份；挂载键缺省表示全部允许项，空数组表示不选挂载。
type BackupCreateReq struct {
	HostID         string   `json:"hostId"`
	ContainerIDs   []string `json:"containerIds"`
	ProjectIDs     []string `json:"projectIds"`
	IncludeData    bool     `json:"includeData"`
	IncludeCompose bool     `json:"includeCompose"`
	IncludeEnv     bool     `json:"includeEnv"`
	StopContainers bool     `json:"stopContainers"`
	MountKeys      []string `json:"mountKeys"`
}

type BackupMount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason"`
}

type BackupCompose struct {
	Project     string   `json:"project"`
	WorkingDir  string   `json:"workingDir"`
	ConfigFiles []string `json:"configFiles"`
	Candidate   bool     `json:"candidate"`
}

type BackupContainer struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Running bool          `json:"running"`
	Mounts  []BackupMount `json:"mounts"`
	Compose BackupCompose `json:"compose"`
}

type BackupResources struct {
	HostID     string            `json:"hostId"`
	Containers []BackupContainer `json:"containers"`
	Projects   []BackupProject   `json:"projects"`
	Limits     BackupLimits      `json:"limits"`
	Warnings   []string          `json:"warnings"`
}

type BackupProject struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Dir   string   `json:"dir"`
	Files []string `json:"files"`
}

type BackupLimits struct {
	MaxArchiveBytes int64 `json:"maxArchiveBytes"`
}

// BackupItem 保留原始来源和归档位置，首版仅提供下载，不自动恢复。
type BackupItem struct {
	Kind        string `json:"kind"`
	ContainerID string `json:"containerId,omitempty"`
	Source      string `json:"source"`
	ArchivePath string `json:"archivePath"`
	MountKey    string `json:"mountKey,omitempty"`
}

type BackupManifest struct {
	ID        string       `json:"id"`
	HostID    string       `json:"hostId"`
	CreatedAt string       `json:"createdAt"`
	Size      int64        `json:"size"`
	SHA256    string       `json:"sha256"`
	Items     []BackupItem `json:"items"`
	Warnings  []string     `json:"warnings"`
}
