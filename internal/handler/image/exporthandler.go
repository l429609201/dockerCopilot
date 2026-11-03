package image

import (
	"fmt"
	"io"
	"net/http"
	"strings"

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

		// 获取主机ID（从查询参数）
		hostID := r.URL.Query().Get("hostId")

		// 按 hostID 路由到对应 Docker 主机
		cli, err := svcCtx.DockerManager.Get(hostID)
		if err != nil {
			logx.Errorf("获取 Docker 客户端失败: %v", err)
			http.Error(w, fmt.Sprintf("主机不可用: %v", err), http.StatusBadRequest)
			return
		}

		// 调用 docker save 导出镜像
		reader, err := cli.ImageSave(r.Context(), []string{req.Id})
		if err != nil {
			logx.Errorf("导出镜像失败 [镜像%s]: %v", req.Id, err)
			http.Error(w, fmt.Sprintf("导出镜像失败: %v", err), http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		// 生成文件名：使用镜像 ID 的前 12 位
		imageIDShort := req.Id
		if strings.HasPrefix(imageIDShort, "sha256:") {
			imageIDShort = strings.TrimPrefix(imageIDShort, "sha256:")
		}
		if len(imageIDShort) > 12 {
			imageIDShort = imageIDShort[:12]
		}
		filename := fmt.Sprintf("image-%s.tar", imageIDShort)

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
