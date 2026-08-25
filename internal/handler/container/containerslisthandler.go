package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ContainersListHandler 获取容器列表
// @Summary 获取容器列表
// @Tags 容器
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /containers [get]
func ContainersListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := container.NewContainersListLogic(r.Context(), svcCtx)
		resp, err := l.ContainersList()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
