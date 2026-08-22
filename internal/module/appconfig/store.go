package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/zeromicro/go-zero/core/logx"
)

// Store 负责 AppConfig 的加载、持久化与并发安全访问。
// 所有对配置的读写都必须经由 Store，禁止直接操作文件。
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  *AppConfig
}

// configPath 返回配置文件路径，优先环境变量 DOCKERCOPILOT_BOT_CONFIG。
func configPath() string {
	if p := os.Getenv("DOCKERCOPILOT_BOT_CONFIG"); p != "" {
		return p
	}
	return "/data/config/config.json"
}

// fileMode 返回 config.json 的文件权限，默认 0666 便于宿主机查看编辑。
func fileMode() os.FileMode {
	if v := os.Getenv("DOCKERCOPILOT_CONFIG_FILE_MODE"); v != "" {
		if parsed, err := strconv.ParseUint(v, 8, 32); err == nil {
			return os.FileMode(parsed)
		}
	}
	return 0666
}

// NewStore 创建配置存储并从磁盘加载；文件不存在时使用默认配置。
func NewStore() *Store {
	s := &Store{path: configPath(), cfg: defaultConfig()}
	s.load()
	return s
}

// load 从磁盘读取配置，失败时保留默认配置并记录日志（不 panic）。
func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			logx.Errorf("读取配置文件失败，使用默认配置: %v", err)
		}
		return
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		logx.Errorf("解析配置文件失败，使用默认配置: %v", err)
		return
	}
	s.cfg = cfg
}

// save 将当前配置写入磁盘（调用方需持有写锁）。
func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	// 先写临时文件再重命名，避免写入过程中崩溃导致配置损坏
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, fileMode()); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Get 返回配置的深拷贝快照，避免外部持有内部引用造成并发问题。
func (s *Store) Get() AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cloneLocked()
}

// cloneLocked 在持锁状态下深拷贝配置。
func (s *Store) cloneLocked() AppConfig {
	data, _ := json.Marshal(s.cfg)
	var clone AppConfig
	_ = json.Unmarshal(data, &clone)
	return clone
}

// Update 以事务方式修改配置：传入的函数对副本进行修改，成功后持久化。
func (s *Store) Update(mutate func(cfg *AppConfig) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	working := s.cloneLocked()
	if err := mutate(&working); err != nil {
		return err
	}
	s.cfg = &working
	return s.save()
}
