package image

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/image"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ImagesListHandler 获取镜像列表
// @Summary 获取镜像列表
// @Tags 镜像
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /images [get]
func ImagesListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := image.NewImagesListLogic(r.Context(), svcCtx)
		resp, err := l.ImagesList()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
