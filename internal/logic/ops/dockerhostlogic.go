package ops

import (
	"context"
	"strings"

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
		views = append(views, hostView{
			ID: h.ID, Name: h.Name, Type: h.Type, Address: h.Address,
			Enabled: h.Enabled, Note: h.Note, Online: online,
			Local: h.ID == appconfig.DockerHostLocalID,
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
					return nil
				}
			}
		}
		newID = uuid.New().String()
		cfg.DockerHosts = append(cfg.DockerHosts, appconfig.DockerHost{
			ID: newID, Name: req.Name, Type: appconfig.DockerHostTypeRemote,
			Address: addr, Enabled: req.Enabled, Note: req.Note,
		})
		return nil
	})
	if err != nil {
		return &types.Resp{Code: 500, Msg: "保存失败：" + err.Error(), Data: map[string]interface{}{}}, nil
	}
	l.svcCtx.ReloadDockerHosts()
	return &types.Resp{Code: 200, Msg: "success", Data: map[string]string{"id": newID}}, nil
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
