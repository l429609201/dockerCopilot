package ops

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/ops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// CreateContainerHandler 从零创建新容器（任务化，返回 taskID 供轮询进度）。
func CreateContainerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateContainerReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		writeResp(w, r, resp, err)
	}
}

// ParseRunCommandHandler 解析 docker run 命令为创建参数（仅解析预览，不创建）。
func ParseRunCommandHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ParseRunCommandReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.ParseRunCommand(&req)
		writeResp(w, r, resp, err)
	}
}
