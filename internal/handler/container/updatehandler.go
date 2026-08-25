package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// UpdateHandler 更新容器
// @Summary 更新容器
// @Tags 容器
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "容器ID"
// @Param containerName formData string true "容器名"
// @Param imageNameAndTag formData string true "镜像名:标签"
// @Success 200 {object} types.Resp "返回 data.taskID"
// @Security BearerAuth
// @Router /container/{id}/update [post]
func UpdateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerUpdateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := container.NewUpdateLogic(r.Context(), svcCtx)
		resp, err := l.Update(&req)
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
