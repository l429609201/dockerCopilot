package ops

import (
	"net/http"

	"github.com/onlyLTY/dockerCopilot/internal/logic/ops"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func writeResp(w http.ResponseWriter, r *http.Request, resp *types.Resp, err error) {
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, resp)
}

// PauseHandler 暂停容器。
func PauseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerActionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewLifecycleLogic(r.Context(), svcCtx)
		resp, err := l.Pause(&req)
		writeResp(w, r, resp, err)
	}
}

// UnpauseHandler 恢复容器。
func UnpauseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerActionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewLifecycleLogic(r.Context(), svcCtx)
		resp, err := l.Unpause(&req)
		writeResp(w, r, resp, err)
	}
}

// KillHandler 强制终止容器。
func KillHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerActionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewLifecycleLogic(r.Context(), svcCtx)
		resp, err := l.Kill(&req)
		writeResp(w, r, resp, err)
	}
}

// RemoveHandler 删除容器。
func RemoveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerRemoveReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewLifecycleLogic(r.Context(), svcCtx)
		resp, err := l.Remove(&req)
		writeResp(w, r, resp, err)
	}
}

// RenameHandler 重命名容器。
func RenameHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerRenameReq2
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := ops.NewLifecycleLogic(r.Context(), svcCtx)
		resp, err := l.Rename(&req)
		writeResp(w, r, resp, err)
	}
}
