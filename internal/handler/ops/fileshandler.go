package ops

import (
	"fmt"
	"net/http"
	"net/url"

	opslogic "github.com/l429609201/dockerCopilot/internal/logic/ops"
	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// FileListHandler 列出容器目录内容。
func FileListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).List(&req)
		writeResp(w, r, resp, err)
	}
}

// FileReadHandler 读取文本文件用于预览/编辑。
func FileReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileReadReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).Read(&req)
		writeResp(w, r, resp, err)
	}
}

// FileWriteHandler 保存文本内容。
func FileWriteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileWriteReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).Write(&req)
		writeResp(w, r, resp, err)
	}
}

// FileMkdirHandler 新建目录。
func FileMkdirHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileMkdirReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).Mkdir(&req)
		writeResp(w, r, resp, err)
	}
}

// FileDeleteHandler 删除文件/目录。
func FileDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileDeleteReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).Delete(&req)
		writeResp(w, r, resp, err)
	}
}

// FileRenameHandler 重命名/移动。
func FileRenameHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileRenameReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := opslogic.NewFilesLogic(r.Context(), svcCtx).Rename(&req)
		writeResp(w, r, resp, err)
	}
}

// errResp 构造失败响应（handler 包内使用）。
func errResp(msg string) *types.Resp {
	return &types.Resp{Code: 400, Msg: msg, Data: map[string]interface{}{}}
}

// FileDownloadHandler 下载文件（返回原始字节流）。
func FileDownloadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FileDownloadReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		name, data, err := containerops.NewForHost(svcCtx, req.HostID).DownloadFile(r.Context(), req.Id, req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(name)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		_, _ = w.Write(data)
	}
}

// FileUploadHandler 上传文件到指定目录（multipart/form-data）。
// 表单字段：path=目标目录，file=文件内容。
func FileUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ContainerActionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, containerops.MaxUploadSize+1024*1024)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			httpx.WriteJson(w, http.StatusOK, errResp("解析上传失败: "+err.Error()))
			return
		}
		dir := r.FormValue("path")
		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.WriteJson(w, http.StatusOK, errResp("读取上传文件失败: "+err.Error()))
			return
		}
		defer file.Close()
		if err := containerops.NewForHost(svcCtx, req.HostID).UploadFile(r.Context(), req.Id, dir, header.Filename, file, 0); err != nil {
			httpx.WriteJson(w, http.StatusOK, errResp(err.Error()))
			return
		}
		httpx.OkJsonCtx(r.Context(), w, &types.Resp{Code: 200, Msg: "success"})
	}
}