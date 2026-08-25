package progress

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/progress"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// GetProgressHandler 查询任务进度
// @Summary 查询任务进度
// @Tags 进度查询
// @Produce json
// @Param taskid path string true "任务ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /progress/{taskid} [get]
func GetProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetProgressReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := progress.NewGetProgressLogic(r.Context(), svcCtx)
		resp, err := l.GetProgress(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
