package version

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/version"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// VersionHandler 获取版本信息
// @Summary 获取版本
// @Tags 版本
// @Produce json
// @Param type query string false "local 或 remote"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /version [get]
func VersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.VersionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := version.NewVersionLogic(r.Context(), svcCtx)
		resp, err := l.Version(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
