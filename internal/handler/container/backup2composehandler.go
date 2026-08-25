package container

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/container"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// Backup2composeHandler 备份为 Compose
// @Summary 备份为 Compose 文件
// @Tags 备份
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /container/backup2compose [get]
func Backup2composeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := container.NewBackup2composeLogic(r.Context(), svcCtx)
		resp, err := l.Backup2compose()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
