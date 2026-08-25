package progress

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/progress"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// CancelProgressHandler 取消指定任务。
// CancelProgressHandler 取消任务
// @Summary 取消任务
// @Tags 进度查询
// @Produce json
// @Param taskid path string true "任务ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /progress/{taskid}/cancel [post]
func CancelProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetProgressReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := progress.NewCancelProgressLogic(r.Context(), svcCtx)
		resp, err := l.CancelProgress(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
