package schedule

import (
	"context"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
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
					cfg.Registries[i].Name = req.Name
					cfg.Registries[i].Registry = req.Registry
					cfg.Registries[i].Username = req.Username
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
			Name:     req.Name,
			Registry: req.Registry,
			Username: req.Username,
			Password: req.Password,
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
