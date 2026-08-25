package version

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/version"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// UpdateProgramHandler 更新程序（已下线）
// @Summary 更新程序（已下线）
// @Tags 版本
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /program [put]
func UpdateProgramHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := version.NewUpdateProgramLogic(r.Context(), svcCtx)
		resp, err := l.UpdateProgram()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
