package ops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/l429609201/dockerCopilot/internal/svc"
)

// ProgressStreamHandler 通过 SSE 持续推送全部后台任务列表。
// 复用 validWSToken 做 query token 鉴权（EventSource 无法带 Authorization 头）。
// 前端任务中心据此实时展示所有任务（更新/恢复/镜像/Compose/定时更新等）。
func ProgressStreamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validWSToken(r, svcCtx.Config.Auth.AccessSecret) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "不支持流式响应", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ctx := r.Context()
		push := func() {
			list := svcCtx.ListProgress()
			data, err := json.Marshal(list)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		push()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				push()
			}
		}
	}
}