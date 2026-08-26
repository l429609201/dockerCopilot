package svc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/zeromicro/go-zero/core/logx"
)

// 远程 Docker 主机的连接层超时。
// 注意：这里刻意不设置 http.Client.Timeout —— 那是请求的墙钟总时长，
// 会把 ImagePull / ContainerLogs / Events 这类长连接流式响应在读 body 途中截断
// （典型报错：context deadline exceeded while reading body）。
// 只约束「建连 / TLS 握手 / 等首个响应头」这三段，主机不可达时能快速失败，
// 而一旦服务端开始正常回流数据，就允许它想传多久传多久。
const (
	remoteDialTimeout           = 10 * time.Second // 建立 TCP 连接
	remoteTLSHandshakeTimeout   = 10 * time.Second // TLS 握手
	remoteResponseHeaderTimeout = 30 * time.Second // 发完请求到收到响应头
)

// newRemoteHTTPClient 构造远程 Docker 主机专用的 HTTP client。
// 超时全部落在 Transport 层，Client.Timeout 保持零值（不限制总时长）。
// Proxy/DisableCompression 与 SDK 默认 Transport 对齐（见 sockets.ConfigureTransport），
// 保证显式传入 client 后代理等行为不变。
func newRemoteHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:              http.ProxyFromEnvironment,
			DisableCompression: false,
			DialContext: (&net.Dialer{
				Timeout:   remoteDialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   remoteTLSHandshakeTimeout,
			ResponseHeaderTimeout: remoteResponseHeaderTimeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}
}

// withRemoteDialer 重设 Transport 的 DialContext。
// 需要它是因为 client.WithHost 内部的 sockets.ConfigureTransport 会把 DialContext
// 覆盖成一个只带 Timeout、没有 KeepAlive 的裸 dialer，因此必须在 WithHost 之后执行。
func withRemoteDialer() client.Opt {
	return func(c *client.Client) error {
		tr, ok := c.HTTPClient().Transport.(*http.Transport)
		if !ok {
			return fmt.Errorf("远程 docker client 的 Transport 类型异常: %T", c.HTTPClient().Transport)
		}
		tr.DialContext = (&net.Dialer{
			Timeout:   remoteDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
		return nil
	}
}

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
	// 远程：指定 tcp:// 地址，超时交给自定义 Transport（见 newRemoteHTTPClient），
	// 不用 client.WithTimeout —— 它设的是请求总时长，会截断镜像拉取等流式响应。
	//
	// opts 顺序有讲究，SDK 按 slice 次序逐个应用：
	//  1. WithHTTPClient 先换上自定义 client，后续 opt 才作用在它身上；
	//  2. WithHost 内部调 sockets.ConfigureTransport 做协议相关配置，
	//     但它会覆盖 DialContext（换成不带 KeepAlive 的裸 dialer）；
	//  3. 所以最后再用 withRemoteDialer 把 dialer 修回来。
	opts := []client.Opt{
		client.WithHTTPClient(newRemoteHTTPClient()),
		client.WithHost(h.Address),
		withRemoteDialer(),
		client.WithAPIVersionNegotiation(),
	}
	// 附加自定义请求头（经反向代理/网关鉴权时携带 Authorization、X-Api-Key 等）
	if len(h.Headers) > 0 {
		opts = append(opts, client.WithHTTPHeaders(h.Headers))
	}
	return client.NewClientWithOpts(opts...)
}

// sameHeaders 判断两个请求头 map 是否完全相等，用于决定连接是否需要重建。
func sameHeaders(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
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
		// 地址与请求头均未变且已有连接则复用，否则重建
		if old, ok := m.hosts[h.ID]; ok && old.Address == h.Address && sameHeaders(old.Headers, h.Headers) && m.clients[h.ID] != nil {
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

// GetClient 返回指定 hostID 的 client。
// hostID 为空视为本地（兼容既有大量以空串代表本地的调用）；
// 指定了具体 hostID 但该主机无可用连接时直接返回 (nil, false)，不回退本地。
//
// 不回退的原因：远程主机建连失败时若静默返回本地 client，
// 读路径会把本地容器伪装成远程主机的容器返回给前端（远程已停止容器"消失"的根因），
// 写路径（start/stop/restart/update/removeimage/prune/backup）更会把操作误打到本地 Docker。
// 失败即失败，由调用方记日志跳过或向用户报错。
func (m *DockerManager) GetClient(hostID string) (*client.Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}
	if cli, ok := m.clients[hostID]; ok && cli != nil {
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
