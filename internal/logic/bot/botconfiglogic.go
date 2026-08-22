package bot

import (
	"context"

	"github.com/onlyLTY/dockerCopilot/internal/module/appconfig"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

// BotConfigLogic 承载 Telegram Bot 配置的读取与更新。
type BotConfigLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotConfigLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotConfigLogic {
	return &BotConfigLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Get 返回脱敏后的 Bot 配置。
func (l *BotConfigLogic) Get() (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	resp.Data = l.svcCtx.AppConfig.MaskedTelegram()
	return resp, nil
}

// Save 更新 Bot 配置并触发 Bot 重载；Token 为空表示保持原值不变。
func (l *BotConfigLogic) Save(req *types.TelegramConfigReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	updateErr := l.svcCtx.AppConfig.Update(func(cfg *appconfig.AppConfig) error {
		cfg.Telegram.Enabled = req.Enabled
		cfg.Telegram.AllowedChatIDs = req.AllowedChatIDs
		cfg.Telegram.Proxy = req.Proxy
		cfg.Telegram.NotifyUpdate = req.NotifyUpdate
		if req.PollIntervalSec > 0 {
			cfg.Telegram.PollIntervalSec = req.PollIntervalSec
		}
		// Token 为空表示不修改，避免脱敏回显后被清空
		if req.Token != "" {
			cfg.Telegram.Token = req.Token
		}
		return nil
	})
	if updateErr != nil {
		resp.Code = 500
		resp.Msg = "保存失败：" + updateErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	// 触发 Bot 按新配置重载
	if l.svcCtx.Bot != nil {
		l.svcCtx.Bot.Reload()
	}
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{}
	return resp, nil
}
