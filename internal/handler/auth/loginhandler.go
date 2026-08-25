package auth

import (
	"github.com/l429609201/dockerCopilot/internal/logic/auth"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

// LoginHandler 登录获取 JWT
// @Summary 登录认证
// @Tags 认证
// @Accept multipart/form-data
// @Produce json
// @Param secretKey formData string true "登录密码"
// @Success 200 {object} types.Resp "返回 data.jwt"
// @Router /auth [post]
func LoginHandler(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.LoginReq
		if err := httpx.Parse(r, &req); err != nil {
			var resp types.Resp
			resp.Code = 400
			resp.Msg = "错误的请求"
			httpx.WriteJson(w, 400, resp)
			return
		}
		l := auth.NewLoginLogic(r.Context(), ctx)
		resp, err := l.Login(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
			return
		}
		httpx.OkJson(w, resp)
	}
}
