package bot

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/bot"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// writeResp 统一响应写出。
func writeResp(w http.ResponseWriter, r *http.Request, resp *types.Resp, err error) {
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, resp)
}

// GetConfigHandler 返回脱敏后的 Bot 配置。
func GetConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := bot.NewBotConfigLogic(r.Context(), svcCtx)
		resp, err := l.Get()
		writeResp(w, r, resp, err)
	}
}

// SaveConfigHandler 更新 Bot 配置并触发重载。
func SaveConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TelegramConfigReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := bot.NewBotConfigLogic(r.Context(), svcCtx)
		resp, err := l.Save(&req)
		writeResp(w, r, resp, err)
	}
}

// TestConfigHandler 发送测试消息，验证 Bot 连通性与白名单可达性。
func TestConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TelegramConfigReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := bot.NewBotConfigLogic(r.Context(), svcCtx)
		resp, err := l.Test(&req)
		writeResp(w, r, resp, err)
	}
}
