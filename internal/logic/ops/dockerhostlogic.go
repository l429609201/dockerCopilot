package ops

import (
	"context"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/system"
	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// DockerHostLogic 承载多 Docker 主机的增删改查与连通性测试。
type DockerHostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDockerHostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DockerHostLogic {
	return &DockerHostLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// hostView 单个主机对外视图，附带在线状态。
type hostView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
	Note    string `json:"note,omitempty"`
	Online  bool   `json:"online"` // 连通性探测结果
	Local   bool   `json:"local"`  // 是否本地主机（前端据此禁用删除/改地址）
	// Headers 自定义请求头，值已脱敏（前端仅用于展示 key 与"已设置"状态）。
	Headers map[string]string `json:"headers,omitempty"`
}

// maskHeaderValue 对请求头值脱敏：非空则统一显示为固定星号串，绝不回显明文凭据。
func maskHeaderValue(v string) string {
	if v == "" {
		return ""
	}
	return "****"
}

// List 返回全部主机及其实时在线状态。
func (l *DockerHostLogic) List() (*types.Resp, error) {
	l.svcCtx.AppConfig.EnsureLocalHost()
	hosts := l.svcCtx.AppConfig.ListDockerHosts()
	views := make([]hostView, 0, len(hosts))
	for _, h := range hosts {
		online := false
		if h.Enabled {
			online = l.svcCtx.DockerManager.Ping(l.ctx, h.ID) == nil
		}
		// 脱敏请求头：只保留 key，值统一星号，避免回显凭据明文
		var maskedHeaders map[string]string
		if len(h.Headers) > 0 {
			maskedHeaders = make(map[string]string, len(h.Headers))
			for k, v := range h.Headers {
				maskedHeaders[k] = maskHeaderValue(v)
			}
		}
		views = append(views, hostView{
			ID: h.ID, Name: h.Name, Type: h.Type, Address: h.Address,
			Enabled: h.Enabled, Note: h.Note, Online: online,
			Local:   h.ID == appconfig.DockerHostLocalID,
			Headers: maskedHeaders,
		})
	}
	return &types.Resp{Code: 200, Msg: "success", Data: views}, nil
}

// Save 新建或更新主机。本地主机仅允许改名与备注；远程主机校验 tcp:// 地址。
func (l *DockerHostLogic) Save(req *types.DockerHostReq) (*types.Resp, error) {
	if strings.TrimSpace(req.Name) == "" {
		return &types.Resp{Code: 400, Msg: "名称不能为空", Data: map[string]interface{}{}}, nil
	}
	// 更新本地主机：只改名/备注/启用，地址与类型强制本地
	if req.ID == appconfig.DockerHostLocalID {
		err := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
			for i := range cfg.DockerHosts {
				if cfg.DockerHosts[i].ID == appconfig.DockerHostLocalID {
					cfg.DockerHosts[i].Name = req.Name
					cfg.DockerHosts[i].Note = req.Note
					cfg.DockerHosts[i].Enabled = true // 本地强制启用
				}
			}
			return nil
		})
		if err != nil {
			return &types.Resp{Code: 500, Msg: "保存失败：" + err.Error(), Data: map[string]interface{}{}}, nil
		}
		l.svcCtx.ReloadDockerHosts()
		return &types.Resp{Code: 200, Msg: "success", Data: map[string]string{"id": appconfig.DockerHostLocalID}}, nil
	}

	// 远程主机：校验地址
	addr := strings.TrimSpace(req.Address)
	if !isValidRemoteAddress(addr) {
		return &types.Resp{Code: 400, Msg: "远程地址需形如 tcp://ip:port", Data: map[string]interface{}{}}, nil
	}

	newID := req.ID
	err := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		if req.ID != "" {
			for i := range cfg.DockerHosts {
				if cfg.DockerHosts[i].ID == req.ID {
					cfg.DockerHosts[i].Name = req.Name
					cfg.DockerHosts[i].Address = addr
					cfg.DockerHosts[i].Enabled = req.Enabled
					cfg.DockerHosts[i].Note = req.Note
					// 合并请求头：空值项保留原值（对应脱敏回显），显式提供的新值覆盖
					cfg.DockerHosts[i].Headers = mergeHeaders(cfg.DockerHosts[i].Headers, req.Headers)
					return nil
				}
			}
		}
		newID = uuid.New().String()
		cfg.DockerHosts = append(cfg.DockerHosts, appconfig.DockerHost{
			ID: newID, Name: req.Name, Type: appconfig.DockerHostTypeRemote,
			Address: addr, Enabled: req.Enabled, Note: req.Note,
			// 新建：过滤掉空值项
			Headers: mergeHeaders(nil, req.Headers),
		})
		return nil
	})
	if err != nil {
		return &types.Resp{Code: 500, Msg: "保存失败：" + err.Error(), Data: map[string]interface{}{}}, nil
	}
	l.svcCtx.ReloadDockerHosts()
	return &types.Resp{Code: 200, Msg: "success", Data: map[string]string{"id": newID}}, nil
}

// mergeHeaders 合并请求头：以 req 为准，但 req 中值为空字符串的项表示「保持原值」。
// 语义：req 里出现的 key 才保留（未出现即视为用户删除）；值非空则覆盖，值为空则沿用 old 中该 key 的原值。
// 用于配合脱敏回显——前端把未修改的头以空值提交，避免把 **** 写回成明文。
func mergeHeaders(old, req map[string]string) map[string]string {
	if len(req) == 0 {
		return nil
	}
	out := make(map[string]string, len(req))
	for k, v := range req {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if v != "" {
			out[k] = v
			continue
		}
		// 空值：沿用原值（若原来就没有则丢弃该空头）
		if ov, ok := old[k]; ok && ov != "" {
			out[k] = ov
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isValidRemoteAddress 校验远程连接地址，仅接受 tcp:// 前缀且含 host。
func isValidRemoteAddress(addr string) bool {
	if !strings.HasPrefix(addr, "tcp://") {
		return false
	}
	return len(strings.TrimPrefix(addr, "tcp://")) > 0
}

// Delete 删除远程主机。本地主机不可删除。
func (l *DockerHostLogic) Delete(req *types.DockerHostIDReq) (*types.Resp, error) {
	if req.ID == appconfig.DockerHostLocalID {
		return &types.Resp{Code: 400, Msg: "本地主机不可删除", Data: map[string]interface{}{}}, nil
	}
	found := false
	err := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		kept := cfg.DockerHosts[:0]
		for _, h := range cfg.DockerHosts {
			if h.ID == req.ID {
				found = true
				continue
			}
			kept = append(kept, h)
		}
		cfg.DockerHosts = kept
		return nil
	})
	if err != nil {
		return &types.Resp{Code: 500, Msg: "删除失败：" + err.Error(), Data: map[string]interface{}{}}, nil
	}
	if !found {
		return &types.Resp{Code: 400, Msg: "主机不存在", Data: map[string]interface{}{}}, nil
	}
	l.svcCtx.ReloadDockerHosts()
	return &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{}}, nil
}

// Ping 测试指定主机连通性。
func (l *DockerHostLogic) Ping(req *types.DockerHostIDReq) (*types.Resp, error) {
	if _, ok := l.svcCtx.AppConfig.FindDockerHost(req.ID); !ok {
		return &types.Resp{Code: 400, Msg: "主机不存在", Data: map[string]interface{}{}}, nil
	}
	if err := l.svcCtx.DockerManager.Ping(l.ctx, req.ID); err != nil {
		return &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{"online": false, "reason": err.Error()}}, nil
	}
	return &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{"online": true}}, nil
}

// hostInfoView Docker 主机详细信息对外视图，字段按前端展示分组。
type hostInfoView struct {
	Online bool `json:"online"`
	// 版本
	DockerVersion string `json:"dockerVersion"`
	APIVersion    string `json:"apiVersion"`
	GoVersion     string `json:"goVersion"`
	GitCommit     string `json:"gitCommit"`
	OsType        string `json:"osType"`
	Architecture  string `json:"architecture"`
	KernelVersion string `json:"kernelVersion"`
	// 运行时
	OperatingSystem   string `json:"operatingSystem"`
	Containers        int    `json:"containers"`
	ContainersRunning int    `json:"containersRunning"`
	ContainersPaused  int    `json:"containersPaused"`
	ContainersStopped int    `json:"containersStopped"`
	Images            int    `json:"images"`
	ContainerdCommit  string `json:"containerdCommit"`
	RuncCommit        string `json:"runcCommit"`
	CgroupVersion     string `json:"cgroupVersion"`
	CgroupDriver      string `json:"cgroupDriver"`
	// 资源
	NCPU          int    `json:"ncpu"`
	MemTotal      int64  `json:"memTotal"`
	DockerRootDir string `json:"dockerRootDir"`
	StorageDriver string `json:"storageDriver"`
	// Registry（运维页面明文展示镜像加速器）
	Mirrors            []string `json:"mirrors"`
	InsecureRegistries []string `json:"insecureRegistries"`
	// 其它
	HostName       string `json:"hostName"`
	DockerName     string `json:"dockerName"`
	SwarmActive    bool   `json:"swarmActive"`
	DefaultRuntime string `json:"defaultRuntime"`
}

// buildHostInfoView 把 docker info + version 组装成前端视图。
// version 可能为零值（获取失败），此时版本类字段用 info.ServerVersion 兜底。
func buildHostInfoView(_ appconfig.DockerHost, info system.Info, ver dockertypes.Version) hostInfoView {
	v := hostInfoView{
		Online:            true,
		DockerVersion:     info.ServerVersion,
		APIVersion:        ver.APIVersion,
		GoVersion:         ver.GoVersion,
		GitCommit:         ver.GitCommit,
		OsType:            info.OSType,
		Architecture:      info.Architecture,
		KernelVersion:     info.KernelVersion,
		OperatingSystem:   info.OperatingSystem,
		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersPaused:  info.ContainersPaused,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,
		ContainerdCommit:  info.ContainerdCommit.ID,
		RuncCommit:        info.RuncCommit.ID,
		CgroupVersion:     info.CgroupVersion,
		CgroupDriver:      info.CgroupDriver,
		NCPU:              info.NCPU,
		MemTotal:          info.MemTotal,
		DockerRootDir:     info.DockerRootDir,
		StorageDriver:     info.Driver,
		HostName:          info.Name,
		DockerName:        info.Name,
		SwarmActive:       info.Swarm.LocalNodeState == "active",
		DefaultRuntime:    info.DefaultRuntime,
	}
	// version 成功时优先用其 Version（与 info.ServerVersion 一致，但更权威）
	if ver.Version != "" {
		v.DockerVersion = ver.Version
	}
	// 镜像加速器 / 非安全 registry：明文展示（运维页面）
	if info.RegistryConfig != nil {
		v.Mirrors = info.RegistryConfig.Mirrors
		for _, cidr := range info.RegistryConfig.InsecureRegistryCIDRs {
			if cidr != nil {
				v.InsecureRegistries = append(v.InsecureRegistries, cidr.String())
			}
		}
	}
	return v
}

// Info 返回指定主机的 Docker 详细信息（docker info + docker version 组合）。
// 供多 Docker 页面「详情」弹窗展示版本、运行时、资源、镜像加速器等。
// 主机离线或无连接时返回可读的错误原因，不视为接口失败（前端据此提示"无法连接"）。
func (l *DockerHostLogic) Info(req *types.DockerHostIDReq) (*types.Resp, error) {
	host, ok := l.svcCtx.AppConfig.FindDockerHost(req.ID)
	if !ok {
		return &types.Resp{Code: 400, Msg: "主机不存在", Data: map[string]interface{}{}}, nil
	}
	cli, ok := l.svcCtx.DockerManager.GetClient(req.ID)
	if !ok || cli == nil {
		return &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{
			"online": false, "reason": "主机无可用连接（未启用或连接失败）",
		}}, nil
	}

	ctx, cancel := context.WithTimeout(l.ctx, 8*time.Second)
	defer cancel()

	info, err := cli.Info(ctx)
	if err != nil {
		return &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{
			"online": false, "reason": "获取信息失败：" + err.Error(),
		}}, nil
	}
	// version 获取失败不阻断：info 里也有 ServerVersion 兜底
	ver, verErr := cli.ServerVersion(ctx)
	if verErr != nil {
		logx.Errorf("获取主机[%s]版本失败: %v", req.ID, verErr)
	}

	detail := buildHostInfoView(host, info, ver)
	return &types.Resp{Code: 200, Msg: "success", Data: detail}, nil
}
