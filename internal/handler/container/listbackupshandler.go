package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListBackupsHandler 获取备份文件列表
// @Summary 获取备份文件列表
// @Tags 备份
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /container/listBackups [get]
func ListBackupsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := container.NewListBackupsLogic(r.Context(), svcCtx)
		resp, err := l.ListBackups()
		if err != nil {
			httpx.WriteJson(w, resp.Code, resp)
		} else {
			httpx.WriteJson(w, resp.Code, resp)
		}
	}
}
