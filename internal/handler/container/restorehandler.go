package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RestoreHandler 恢复容器
// @Summary 恢复容器
// @Tags 备份
// @Accept json
// @Produce json
// @Param body body types.ContainerRestoreReq true "备份文件名"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /container/backups/restore [post]
func RestoreHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerRestoreReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := container.NewRestoreLogic(r.Context(), svcCtx)
		resp, err := l.Restore(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
