package schedule

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/schedule"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegistryListHandler 返回脱敏后的 Registry 凭据列表。
// RegistryListHandler 获取 Registry 凭据列表
// @Summary 获取凭据列表
// @Tags Registry 凭据
// @Produce json
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /registries [get]
func RegistryListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := schedule.NewRegistryLogic(r.Context(), svcCtx)
		resp, err := l.List()
		writeResp(w, r, resp, err)
	}
}

// RegistrySaveHandler 新建或更新 Registry 凭据。
// RegistrySaveHandler 创建/更新 Registry 凭据
// @Summary 创建或更新凭据
// @Tags Registry 凭据
// @Accept json
// @Produce json
// @Param body body types.RegistryReq true "凭据信息"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /registries [post]
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
// RegistryDeleteHandler 删除 Registry 凭据
// @Summary 删除凭据
// @Tags Registry 凭据
// @Produce json
// @Param id path string true "凭据ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /registries/{id} [delete]
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

// RegistryRateLimitHandler 查询指定凭据在 Docker Hub 的剩余拉取次数。
// RegistryRateLimitHandler 查询 Docker Hub 拉取次数配额
// @Summary 查询拉取次数配额
// @Tags Registry 凭据
// @Produce json
// @Param id path string true "凭据ID"
// @Success 200 {object} types.Resp
// @Security BearerAuth
// @Router /registries/{id}/ratelimit [get]
func RegistryRateLimitHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RegistryIDReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := schedule.NewRegistryLogic(r.Context(), svcCtx)
		resp, err := l.RateLimit(&req)
		writeResp(w, r, resp, err)
	}
}
