package ops

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/ops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// HostPathConfigGetHandler 获取宿主机路径映射配置（含自动推导预览）。
func HostPathConfigGetHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		httpx.OkJsonCtx(r.Context(), w, logic.GetConfig())
	}
}

// HostPathConfigSaveHandler 保存宿主机路径映射配置。
func HostPathConfigSaveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.HostPathConfigReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.SaveConfig(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathResolveHandler 解析容器路径到宿主机路径并校验可访问性。
func HostPathResolveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.HostPathResolveReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.ValidateAndResolve(req.ContainerPath)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, &types.Resp{Code: 400, Msg: err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

