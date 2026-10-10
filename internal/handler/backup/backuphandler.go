package backup

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	backupmod "github.com/l429609201/dockerCopilot/internal/module/backup"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// respond 沿用现有 API envelope，下载成功时直接返回归档流。
func respond(w http.ResponseWriter, data any, err error, status int) {
	if err != nil {
		httpx.WriteJson(w, status, types.Resp{Code: status, Msg: err.Error()})
		return
	}
	httpx.WriteJson(w, http.StatusOK, types.Resp{Code: http.StatusOK, Msg: "success", Data: data})
}

func ResourcesHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := backupmod.New(s).Resources(r.Context(), r.URL.Query().Get("hostId"))
		respond(w, data, err, http.StatusBadRequest)
	}
}

func CreateHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.BackupCreateReq
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			respond(w, nil, err, http.StatusBadRequest)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			respond(w, nil, errors.New("请求必须只有一个 JSON 对象"), http.StatusBadRequest)
			return
		}
		id, err := backupmod.New(s).Create(r.Context(), req)
		respond(w, map[string]string{"taskID": id}, err, http.StatusBadRequest)
	}
}

func ListHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := backupmod.New(s).List()
		respond(w, data, err, http.StatusInternalServerError)
	}
}

func DownloadHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `path:"id"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			respond(w, nil, err, http.StatusBadRequest)
			return
		}
		file, manifest, err := backupmod.New(s).Open(req.ID)
		if err != nil {
			status := http.StatusBadRequest
			if os.IsNotExist(err) {
				status = http.StatusNotFound
			}
			respond(w, nil, err, status)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			respond(w, nil, err, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+manifest.ID+".tar.gz\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, manifest.ID+".tar.gz", info.ModTime(), file)
	}
}

func DeleteHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `path:"id"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			respond(w, nil, err, http.StatusBadRequest)
			return
		}
		err := backupmod.New(s).Delete(req.ID)
		respond(w, map[string]string{"id": req.ID}, err, http.StatusBadRequest)
	}
}
