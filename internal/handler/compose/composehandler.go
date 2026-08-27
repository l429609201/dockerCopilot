package compose

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/compose"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
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

// CreateHandler 从内容创建并部署一个新的 Compose 项目。
func CreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeCreateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		writeResp(w, r, resp, err)
	}
}

// BrowseHandler 浏览 DC 自身文件系统的目录（供前端目录选择器使用，只读）。
func BrowseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeBrowseReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.Browse(&req)
		writeResp(w, r, resp, err)
	}
}

// GetConfigHandler 返回当前生效的 Compose 扫描配置（供前端配置卡片回显）。
func GetConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.GetConfig()
		writeResp(w, r, resp, err)
	}
}

// SaveConfigHandler 保存 Compose 扫描配置到持久化存储。
func SaveConfigHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeConfigReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.SaveConfig(&req)
		writeResp(w, r, resp, err)
	}
}

// CreateFolderHandler 在指定目录下创建文件夹。
func CreateFolderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeCreateFolderReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.CreateFolder(&req)
		writeResp(w, r, resp, err)
	}
}

// CreateComposeFileHandler 在指定目录下创建 Compose 配置文件。
func CreateComposeFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeCreateFileReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.CreateComposeFile(&req)
		writeResp(w, r, resp, err)
	}
}

// ReadFileHandler 读取文件内容。
func ReadFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeReadFileReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.ReadFile(&req)
		writeResp(w, r, resp, err)
	}
}

// SaveFileHandler 保存文件内容。
func SaveFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ComposeSaveFileReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := compose.NewComposeLogic(r.Context(), svcCtx)
		resp, err := l.SaveFile(&req)
		writeResp(w, r, resp, err)
	}
}
