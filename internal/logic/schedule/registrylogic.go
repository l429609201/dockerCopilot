package schedule

import (
	"context"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// RegistryLogic 承载 Registry 凭据的增删改查。
// 所有对外返回的凭据均经过脱敏，绝不回显明文密码。
type RegistryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegistryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegistryLogic {
	return &RegistryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 返回脱敏后的凭据列表。
func (l *RegistryLogic) List() (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	resp.Data = l.svcCtx.AppConfig.MaskedRegistries()
	return resp, nil
}

// Save 新建或更新凭据；更新时 Password 为空表示保持原密码不变。
func (l *RegistryLogic) Save(req *types.RegistryReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Name == "" || req.Username == "" {
		resp.Code = 400
		resp.Msg = "名称和用户名不能为空"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	newID := req.ID
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		if req.ID != "" {
			for i := range cfg.Registries {
				if cfg.Registries[i].ID == req.ID {
					cfg.Registries[i].Type = normalizeRegistryType(req.Type)
					cfg.Registries[i].Name = req.Name
					cfg.Registries[i].Registry = req.Registry
					cfg.Registries[i].Username = req.Username
					cfg.Registries[i].Note = req.Note
					// 密码为空表示不修改，保留原值，避免脱敏回显后被清空
					if req.Password != "" {
						cfg.Registries[i].Password = req.Password
					}
					return nil
				}
			}
		}
		newID = uuid.New().String()
		cfg.Registries = append(cfg.Registries, appconfig.RegistryCredential{
			ID:       newID,
			Type:     normalizeRegistryType(req.Type),
			Name:     req.Name,
			Registry: req.Registry,
			Username: req.Username,
			Password: req.Password,
			Note:     req.Note,
		})
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "保存失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"id": newID}
	return resp, nil
}

// normalizeRegistryType 归一化凭据类型，非法或空值统一为 custom。
func normalizeRegistryType(t string) string {
	switch t {
	case "dockerhub", "github", "custom":
		return t
	default:
		return "custom"
	}
}

// Delete 删除指定凭据。
func (l *RegistryLogic) Delete(req *types.RegistryIDReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	found := false
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		kept := cfg.Registries[:0]
		for _, r := range cfg.Registries {
			if r.ID == req.ID {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		cfg.Registries = kept
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "删除失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if !found {
		resp.Code = 400
		resp.Msg = "凭据不存在"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp, nil
}

// RateLimit 查询指定凭据在 Docker Hub 的剩余拉取次数。
// 仅 dockerhub 类型（或地址指向 Docker Hub）支持；其他类型返回"不适用"。
func (l *RegistryLogic) RateLimit(req *types.RegistryIDReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	cred, ok := l.svcCtx.AppConfig.FindRegistry(req.ID)
	if !ok {
		resp.Code = 400
		resp.Msg = "凭据不存在"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	// 仅 Docker Hub 具备拉取次数配额；其他 registry 不适用
	if !isDockerHubCred(&cred) {
		resp.Data = types.RegistryRateLimitResp{Supported: false, Message: "该仓库类型无拉取次数限制"}
		return resp, nil
	}
	rl, qErr := utiles.CheckDockerHubRateLimit(l.ctx, &cred)
	if qErr != nil {
		resp.Data = types.RegistryRateLimitResp{Supported: false, Message: "查询失败：" + qErr.Error()}
		return resp, nil
	}
	resp.Data = types.RegistryRateLimitResp{
		Supported: true,
		Limit:     rl.Limit,
		Remaining: rl.Remaining,
		Source:    rl.Source,
	}
	return resp, nil
}

// isDockerHubCred 判断凭据是否指向 Docker Hub。
// dockerhub 类型，或地址为空/显式 docker.io 均视为 Docker Hub。
func isDockerHubCred(cred *appconfig.RegistryCredential) bool {
	if cred.Type == "dockerhub" {
		return true
	}
	if cred.Type == "" {
		reg := cred.Registry
		return reg == "" || reg == "docker.io" || reg == "registry-1.docker.io" || reg == "https://index.docker.io/v1/"
	}
	return false
}
