package ops

import (
	"net/http"

	"github.com/l429609201/dockerCopilot/internal/logic/ops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// HostPathResolveHandler 解析容器路径到宿主机路径。
func HostPathResolveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.HostPathResolveReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		hostPath, err := logic.ResolveHostPath(req.ContainerPath)
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  err.Error(),
			})
			return
		}

		httpx.OkJsonCtx(r.Context(), w, &types.Resp{
			Code: 200,
			Msg:  "success",
			Data: map[string]interface{}{
				"containerPath": req.ContainerPath,
				"hostPath":      hostPath,
			},
		})
	}
}

// HostPathListHandler 列出映射目录内容。
func HostPathListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.HostPathListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.ListMappedDir(req.Path)
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  err.Error(),
			})
			return
		}

		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathMappingsHandler 获取所有路径映射配置。
func HostPathMappingsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp := logic.GetMappings()
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathReadHandler 读取文件内容
func HostPathReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  "path 参数不能为空",
			})
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.ReadFile(path)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathWriteHandler 写入文件
func HostPathWriteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.WriteFile(req.Path, req.Content)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathMkdirHandler 创建目录
func HostPathMkdirHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.CreateDir(req.Path)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathDeleteHandler 删除文件或目录
func HostPathDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.DeletePath(req.Path)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathRenameHandler 重命名或移动文件
func HostPathRenameHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OldPath string `json:"oldPath"`
			NewPath string `json:"newPath"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.RenamePath(req.OldPath, req.NewPath)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// HostPathDownloadHandler 下载文件
func HostPathDownloadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  "path 参数不能为空",
			})
			return
		}

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		content, filename, err := logic.DownloadFile(path)
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		w.Write(content)
	}
}

// HostPathUploadHandler 上传文件
func HostPathUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(32 << 20) // 32MB
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  "解析上传文件失败: " + err.Error(),
			})
			return
		}

		path := r.FormValue("path")
		file, handler, err := r.FormFile("file")
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, &types.Resp{
				Code: 400,
				Msg:  "获取上传文件失败: " + err.Error(),
			})
			return
		}
		defer file.Close()

		logic := ops.NewHostPathMapperLogic(r.Context(), svcCtx)
		resp, err := logic.UploadFile(path, handler.Filename, file)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, resp)
			return
		}

		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
