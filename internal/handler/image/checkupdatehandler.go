package image

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// CheckUpdateHandler 手动触发检测所有镜像更新
// @Summary 检测镜像更新
// @Tags 镜像
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /images/check-update [post]
func CheckUpdateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := image.NewCheckUpdateLogic(r.Context(), svcCtx)
		resp, err := l.CheckUpdate()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
