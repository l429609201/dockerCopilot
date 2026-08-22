package compose

import (
	"net/http"

	"github.com/onlyLTY/dockerCopilot/internal/logic/compose"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// writeResp 统一响应写出。
func writeResp(w http.ResponseWriter, r *http.Request, resp *types.Resp, err error) {
	if err != nil {
		httpx.WriteJson(w, resp.Code, resp)
		return
	}
	httpx.OkJsonCtx(r.Context(), w, resp)
}

// ListHandler 返回所有 Compose 项目。
func ListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.List()
		writeResp(w, r, resp, err)
	}
}

// ReadFileHandler 读取项目文件内容。
func ReadFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeFileReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.ReadFile(&req)
		writeResp(w, r, resp, err)
	}
}

// SaveFileHandler 保存项目文件内容。
func SaveFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeFileSaveReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.SaveFile(&req)
		writeResp(w, r, resp, err)
	}
}

// ValidateHandler 校验 compose 内容。
func ValidateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeValidateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.Validate(&req)
		writeResp(w, r, resp, err)
	}
}

// ActionHandler 执行部署类操作。
func ActionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeActionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.Action(&req)
		writeResp(w, r, resp, err)
	}
}
