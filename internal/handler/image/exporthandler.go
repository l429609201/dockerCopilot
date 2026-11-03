package image

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/handler/ops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ExportHandler 导出镜像为 tar 文件
// @Summary 导出镜像为 tar 文件
// @Tags 镜像
// @Produce application/x-tar
// @Param id path string true "镜像ID"
// @Param hostId query string false "主机ID"
// @Success 200 {file} binary "镜像 tar 文件"
// @Security BearerAuth
// @Router /image/{id}/export [get]
func ExportHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ExportImageReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 浏览器 <a> 下载无法携带 Authorization 头，路由不走 WithJwt，
		// 改为与 SSE/WebSocket 接口一致的 query token 校验（也兼容 Authorization 头）。
		if !ops.ValidQueryToken(r, svcCtx.Config.Auth.AccessSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if req.Id == "" {
			http.Error(w, "镜像ID不能为空", http.StatusBadRequest)
			return
		}

		// 按 hostID 路由到对应 Docker 主机（空表示本地）
		cli, ok := svcCtx.DockerManager.GetClient(req.HostID)
		if !ok {
			logx.Errorf("导出镜像失败：Docker 主机 %s 不可用", req.HostID)
			http.Error(w, "Docker 主机不可用", http.StatusBadRequest)
			return
		}

		// 导出目标：有引用时按「名字:标签」导出，tar 的 manifest.json 会写入 RepoTags，
		// 目标机 docker load 后自动带标签；按 ID 导出则 load 后是 <none>:<none>。
		saveTarget := req.Id
		ref := strings.TrimSpace(req.Ref)
		if ref != "" && !strings.Contains(ref, "<none>") {
			// 校验引用当前确实指向该镜像，防止标签已被移到别的镜像时导出错内容
			inspect, _, ierr := cli.ImageInspectWithRaw(r.Context(), ref)
			if ierr != nil {
				http.Error(w, fmt.Sprintf("镜像引用 %s 不存在: %v", ref, ierr), http.StatusBadRequest)
				return
			}
			if !sameImageID(inspect.ID, req.Id) {
				http.Error(w, fmt.Sprintf("镜像引用 %s 已不指向该镜像，请刷新列表后重试", ref), http.StatusConflict)
				return
			}
			saveTarget = ref
		}

		// 调用 docker save 导出镜像
		reader, err := cli.ImageSave(r.Context(), []string{saveTarget})
		if err != nil {
			logx.Errorf("导出镜像失败 [镜像%s]: %v", saveTarget, err)
			http.Error(w, fmt.Sprintf("导出镜像失败: %v", err), http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		filename := exportFilename(saveTarget, req.Id)

		// 设置响应头
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")

		// 流式传输镜像数据
		written, err := copyWithFlush(w, reader)
		if err != nil {
			logx.Errorf("传输镜像数据失败 [已传输%d字节]: %v", written, err)
			return
		}

		logx.Infof("镜像导出完成 [镜像%s] [大小%d字节]", req.Id, written)
	}
}

// sameImageID 比较两个镜像ID，兼容带/不带 "sha256:" 前缀以及短ID。
func sameImageID(a, b string) bool {
	a = strings.TrimPrefix(a, "sha256:")
	b = strings.TrimPrefix(b, "sha256:")
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// exportFilename 生成下载文件名：按引用导出时用「名字_标签.tar」，否则用镜像ID前12位。
// 只保留安全字符，避免引号、斜杠等破坏 Content-Disposition 头。
func exportFilename(saveTarget, id string) string {
	if saveTarget != id {
		var b strings.Builder
		for _, c := range saveTarget {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
				b.WriteRune(c)
			default:
				b.WriteByte('_')
			}
		}
		return b.String() + ".tar"
	}
	short := strings.TrimPrefix(id, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	return "image-" + short + ".tar"
}

// copyWithFlush 复制数据并定期刷新缓冲区，确保大文件流式传输
func copyWithFlush(w http.ResponseWriter, reader io.Reader) (int64, error) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	var written int64
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			nw, writeErr := w.Write(buf[:n])
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if nw != n {
				return written, io.ErrShortWrite
			}
			// 每 32KB 刷新一次
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return written, readErr
		}
	}
	return written, nil
}
