package svc

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/zeromicro/go-zero/core/logx"
)

// DockerManager 管理多个 Docker 主机的 client 连接池。
// 内部按 hostID 维护 *client.Client；本地主机 ID 恒为 appconfig.DockerHostLocalID。
// 旧代码通过 svcCtx.DockerClient 访问的仍是本地 client，无需改动即默认指向本地。
type DockerManager struct {
	mu      sync.RWMutex
	clients map[string]*client.Client
	hosts   map[string]appconfig.DockerHost
}

// NewDockerManager 按主机列表创建连接池。本地主机必定存在（由 appconfig.EnsureLocalHost 保证）。
func NewDockerManager(hosts []appconfig.DockerHost) *DockerManager {
	m := &DockerManager{
		clients: make(map[string]*client.Client),
		hosts:   make(map[string]appconfig.DockerHost),
	}
	m.Reload(hosts)
	return m
}

// newClientForHost 按主机配置创建单个 docker client。
// 本地走 FromEnv（沿用原行为，兼容各种运行环境）；远程走 WithHost(tcp://...)。
func newClientForHost(h appconfig.DockerHost) (*client.Client, error) {
	if h.Type == appconfig.DockerHostTypeLocal || h.Address == "" || h.Address == appconfig.LocalDockerAddress {
		// 本地：沿用 FromEnv，兼容各种运行环境
		return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}
	// 远程：指定 tcp:// 地址并设置整体超时，避免不可达主机长时间阻塞
	return client.NewClientWithOpts(
		client.WithHost(h.Address),
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(10*time.Second),
	)
}

// Reload 按最新主机列表重建连接池：新增/变更的主机重建 client，移除的主机关闭 client。
func (m *DockerManager) Reload(hosts []appconfig.DockerHost) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newClients := make(map[string]*client.Client, len(hosts))
	newHosts := make(map[string]appconfig.DockerHost, len(hosts))
	for _, h := range hosts {
		newHosts[h.ID] = h
		if !h.Enabled {
			continue // 禁用主机不建连接
		}
		// 地址未变且已有连接则复用
		if old, ok := m.hosts[h.ID]; ok && old.Address == h.Address && m.clients[h.ID] != nil {
			newClients[h.ID] = m.clients[h.ID]
			continue
		}
		cli, err := newClientForHost(h)
		if err != nil {
			logx.Errorf("创建 Docker 主机[%s:%s] client 失败: %v", h.ID, h.Address, err)
			continue
		}
		newClients[h.ID] = cli
	}
	// 关闭已不再使用的旧连接（远程连接，避免泄漏；本地 FromEnv 也一并按需重建）
	for id, cli := range m.clients {
		if _, kept := newClients[id]; !kept && cli != nil {
			_ = cli.Close()
		}
	}
	m.clients = newClients
	m.hosts = newHosts
}

// GetClient 返回指定 hostID 的 client。hostID 为空或未找到时回退本地 client。
func (m *DockerManager) GetClient(hostID string) (*client.Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	if cli, ok := m.clients[hostID]; ok && cli != nil {
		return cli, true
	}
	// 回退本地
	if cli, ok := m.clients[appconfig.DockerHostLocalID]; ok && cli != nil {
		return cli, true
	}
	return nil, false
}

// Local 返回本地 client，供 svcCtx.DockerClient 兼容赋值。
func (m *DockerManager) Local() *client.Client {
	cli, _ := m.GetClient(appconfig.DockerHostLocalID)
	return cli
}

// EnabledHosts 返回当前启用且成功建连的主机列表（按传入顺序无法保证，调用方需要顺序时自行排序）。
func (m *DockerManager) EnabledHosts() []appconfig.DockerHost {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]appconfig.DockerHost, 0, len(m.hosts))
	for id, h := range m.hosts {
		if h.Enabled && m.clients[id] != nil {
			out = append(out, h)
		}
	}
	return out
}

// HostCode 返回主机在 Telegram callback 中使用的短码，用于绕过 callback data 64 字节限制。
// 本地主机固定为 "L"；远程主机按 hosts map 的稳定排序取序号 "0"/"1"/...。
// hostID 为空或本地时返回 "L"。
func (m *DockerManager) HostCode(hostID string) string {
	if hostID == "" || hostID == appconfig.DockerHostLocalID {
		return "L"
	}
	for i, id := range m.sortedRemoteIDs() {
		if id == hostID {
			return strconv.Itoa(i)
		}
	}
	return "L"
}

// HostByCode 将 callback 短码还原为 hostID。"L"/空/非法 → 本地。
func (m *DockerManager) HostByCode(code string) string {
	if code == "" || code == "L" {
		return appconfig.DockerHostLocalID
	}
	idx, err := strconv.Atoi(code)
	if err != nil {
		return appconfig.DockerHostLocalID
	}
	remotes := m.sortedRemoteIDs()
	if idx < 0 || idx >= len(remotes) {
		return appconfig.DockerHostLocalID
	}
	return remotes[idx]
}

// sortedRemoteIDs 返回除本地外的远程主机 ID，按字典序稳定排序，保证短码映射稳定。
func (m *DockerManager) sortedRemoteIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.hosts))
	for id := range m.hosts {
		if id != appconfig.DockerHostLocalID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Ping 测试指定主机连通性；hostID 为空测试本地。
func (m *DockerManager) Ping(ctx context.Context, hostID string) error {
	cli, ok := m.GetClient(hostID)
	if !ok {
		return fmt.Errorf("主机 %s 无可用连接", hostID)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := cli.Ping(pingCtx)
	return err
}
