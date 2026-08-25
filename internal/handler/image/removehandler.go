package image

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RemoveHandler 删除镜像
// @Summary 删除镜像
// @Tags 镜像
// @Produce json
// @Param id path string true "镜像ID"
// @Param force query boolean false "强制删除"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /image/{id} [delete]
func RemoveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RemoveImageReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := image.NewRemoveLogic(r.Context(), svcCtx)
		resp, err := l.Remove(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
