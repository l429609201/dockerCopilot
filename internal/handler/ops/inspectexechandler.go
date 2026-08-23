package ops

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/ops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// InspectHandler 返回容器完整配置。
func InspectHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerInspectReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewInspectExecLogic(r.Context(), svcCtx)
		resp, err := l.Inspect(&req)
		writeResp(w, r, resp, err)
	}
}

// LogsHandler 返回容器日志。
func LogsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerLogsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewInspectExecLogic(r.Context(), svcCtx)
		resp, err := l.Logs(&req)
		writeResp(w, r, resp, err)
	}
}

// ExecHandler 在容器内执行命令。
func ExecHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerExecReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewInspectExecLogic(r.Context(), svcCtx)
		resp, err := l.Exec(&req)
		writeResp(w, r, resp, err)
	}
}
