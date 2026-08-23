package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
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

// xorObfuscate 用 key 对 data 逐字节 XOR 后做 Base64 编码。
// key 为空或 data 为空时返回空串。用于将明文 Token 混淆后回传给已登录前端，
// 前端持有同一 key（登录 JWT 令牌字符串）即可解回明文，服务器签名密钥不出后端。
func xorObfuscate(data, key string) string {
	if data == "" || key == "" {
		return ""
	}
	kb := []byte(key)
	db := []byte(data)
	out := make([]byte, len(db))
	for i := 0; i < len(db); i++ {
		out[i] = db[i] ^ kb[i%len(kb)]
	}
	return base64.StdEncoding.EncodeToString(out)
}

// Get 返回脱敏后的 Bot 配置；jwtToken 非空时额外返回 XOR 混淆后的明文 Token。
func (l *BotConfigLogic) Get(jwtToken string) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success"}
	data := l.svcCtx.AppConfig.MaskedTelegram()
	// 用登录令牌作密钥混淆明文 Token，供前端"点击查看"解码，避免明文直接暴露在响应中
	if jwtToken != "" {
		data["tokenObf"] = xorObfuscate(l.svcCtx.AppConfig.RawTelegramToken(), jwtToken)
	}
	resp.Data = data
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
		// UpdateCheckIntervalMinutes：更新检测周期(分钟)，<=0 时 notifier 会用默认 30
		cfg.Telegram.UpdateCheckIntervalMinutes = req.UpdateCheckIntervalMinutes
		// MutedContainers：更新检查屏蔽黑名单，命中的容器不推送"有更新"通知
		cfg.Telegram.MutedContainers = req.MutedContainers
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

// Test 发送一条测试消息，验证 Token、代理和白名单 Chat ID 是否可用。
// Token 为空时使用已保存的配置，便于用户在不重填 Token 的情况下测试连通性。
func (l *BotConfigLogic) Test(req *types.TelegramConfigReq) (resp *types.Resp, err error) {
	resp = &types.Resp{Code: 200, Msg: "success", Data: map[string]interface{}{}}
	cfg := l.svcCtx.AppConfig.Get().Telegram

	// Token：请求传入优先，否则用已存明文
	token := req.Token
	if token == "" {
		token = cfg.Token
	}
	if token == "" {
		resp.Code = 400
		resp.Msg = "未配置 Bot Token，无法测试"
		return resp, nil
	}

	// 代理：请求传入优先，否则沿用已存配置
	proxy := req.Proxy
	if proxy == "" {
		proxy = cfg.Proxy
	}

	// 白名单：请求传入优先，否则用已存配置
	chatIDs := req.AllowedChatIDs
	if len(chatIDs) == 0 {
		chatIDs = cfg.AllowedChatIDs
	}
	if len(chatIDs) == 0 {
		resp.Code = 400
		resp.Msg = "未配置白名单 Chat ID，无法投递测试消息"
		return resp, nil
	}

	client, e := telegram.NewClient(token, proxy)
	if e != nil {
		resp.Code = 500
		resp.Msg = "创建 Telegram 客户端失败：" + e.Error()
		return resp, nil
	}

	text := fmt.Sprintf("<b>dockerCopilot 测试消息</b>\n连接正常，时间：%s",
		time.Now().Format("2006-01-02 15:04:05"))
	var okCount int
	var lastErr error
	for _, chatID := range chatIDs {
		if err := client.SendMessage(chatID, text, nil); err != nil {
			lastErr = err
			logx.Errorf("Telegram 测试消息发送失败 chat=%d: %v", chatID, err)
			continue
		}
		okCount++
	}

	if okCount == 0 {
		resp.Code = 500
		resp.Msg = "测试消息发送失败：" + errText(lastErr)
		return resp, nil
	}
	resp.Msg = fmt.Sprintf("测试消息已发送（成功 %d/%d 个会话）", okCount, len(chatIDs))
	return resp, nil
}

// errText 安全地取错误文本。
func errText(err error) string {
	if err == nil {
		return "未知错误"
	}
	return err.Error()
}
