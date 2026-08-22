package schedule

import (
	"net/http"

	"github.com/onlyLTY/dockerCopilot/internal/logic/schedule"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegistryListHandler 返回脱敏后的 Registry 凭据列表。
func RegistryListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := schedule.NewRegistryLogic(r.Context(), svcCtx)
		resp, err := l.List()
		writeResp(w, r, resp, err)
	}
}

// RegistrySaveHandler 新建或更新 Registry 凭据。
func RegistrySaveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RegistryReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewRegistryLogic(r.Context(), svcCtx)
		resp, err := l.Save(&req)
		writeResp(w, r, resp, err)
	}
}

// RegistryDeleteHandler 删除 Registry 凭据。
func RegistryDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RegistryIDReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewRegistryLogic(r.Context(), svcCtx)
		resp, err := l.Delete(&req)
		writeResp(w, r, resp, err)
	}
}
