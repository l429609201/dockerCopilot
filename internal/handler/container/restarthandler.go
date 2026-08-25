package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RestartHandler 重启容器
// @Summary 重启容器
// @Tags 容器
// @Produce json
// @Param id path string true "容器ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /container/{id}/restart [post]
func RestartHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IdReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := container.NewRestartLogic(r.Context(), svcCtx)
		resp, err := l.Restart(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
