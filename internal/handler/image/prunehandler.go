package image

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// PruneHandler 提交异步批量清理镜像任务。
func PruneHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PruneImagesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := image.NewPruneLogic(r.Context(), svcCtx)
		resp, err := l.Prune(&req)
		if err != nil {
			httpx.WriteJson(w, http.StatusInternalServerError, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
