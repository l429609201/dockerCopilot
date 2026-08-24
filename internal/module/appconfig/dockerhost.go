package appconfig

// LocalDockerAddress 本地主机固定连接地址（unix socket），不可编辑。
const LocalDockerAddress = "unix:///var/run/docker.sock"

// defaultLocalHost 返回默认的本地主机配置。
func defaultLocalHost() DockerHost {
	return DockerHost{
		ID:      DockerHostLocalID,
		Name:    "本地 Docker",
		Type:    DockerHostTypeLocal,
		Address: LocalDockerAddress,
		Enabled: true,
	}
}

// EnsureLocalHost 保证 DockerHosts 首项为本地主机（幂等）。
// 本地主机不存在时插入到列表首位；已存在则强制其 Type/Address 为本地固定值，
// 允许用户改名，但不允许改地址或类型。返回是否发生了变更。
func (s *Store) EnsureLocalHost() (changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hosts := s.cfg.DockerHosts
	localIdx := -1
	for i := range hosts {
		if hosts[i].ID == DockerHostLocalID {
			localIdx = i
			break
		}
	}
	if localIdx < 0 {
		// 本地主机缺失：插入到首位
		s.cfg.DockerHosts = append([]DockerHost{defaultLocalHost()}, hosts...)
		_ = s.save()
		return true
	}
	// 已存在：矫正为本地固定值（保留用户自定义名称）
	h := &s.cfg.DockerHosts[localIdx]
	if h.Type != DockerHostTypeLocal || h.Address != LocalDockerAddress || !h.Enabled {
		h.Type = DockerHostTypeLocal
		h.Address = LocalDockerAddress
		h.Enabled = true
		changed = true
	}
	if h.Name == "" {
		h.Name = "本地 Docker"
		changed = true
	}
	// 本地主机若不在首位，移动到首位，保证展示顺序稳定
	if localIdx != 0 {
		local := s.cfg.DockerHosts[localIdx]
		s.cfg.DockerHosts = append(s.cfg.DockerHosts[:localIdx], s.cfg.DockerHosts[localIdx+1:]...)
		s.cfg.DockerHosts = append([]DockerHost{local}, s.cfg.DockerHosts...)
		changed = true
	}
	if changed {
		_ = s.save()
	}
	return changed
}

// ListDockerHosts 返回全部 Docker 主机的副本切片。
func (s *Store) ListDockerHosts() []DockerHost {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DockerHost, len(s.cfg.DockerHosts))
	copy(out, s.cfg.DockerHosts)
	return out
}

// FindDockerHost 按 ID 查找主机，返回副本与是否存在。
func (s *Store) FindDockerHost(id string) (DockerHost, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range s.cfg.DockerHosts {
		if h.ID == id {
			return h, true
		}
	}
	return DockerHost{}, false
}
