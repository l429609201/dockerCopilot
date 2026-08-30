package ops

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// LogsStreamHandler 通过 SSE 流式推送容器日志：逐行边读边下发，首行秒级到达。
// 支持后端关键词过滤（search，等效 docker logs | grep）与实时跟随（follow，等效 -f）。
// EventSource 无法带 Authorization 头，故用 query token 校验（复用 validWSToken）。
func LogsStreamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validWSToken(r, svcCtx.Config.Auth.AccessSecret) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		var req types.ContainerLogsStreamReq
		if err := httpx.Parse(r, &req); err != nil {
			http.Error(w, "参数错误: "+err.Error(), http.StatusBadRequest)
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
		w.Header().Set("X-Accel-Buffering", "no") // 关闭 Nginx 缓冲，保证实时下发

		ctx := r.Context()

		// sendEvent 按 SSE 规范下发一条事件。日志行可能含换行，
		// 需把内部换行拆成多条 data: 行（SSE 规定多行 data 以 \n 拼接为一条消息）。
		sendEvent := func(event, payload string) {
			if event != "" {
				fmt.Fprintf(w, "event: %s\n", event)
			}
			for _, ln := range strings.Split(payload, "\n") {
				fmt.Fprintf(w, "data: %s\n", ln)
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}

		svc := containerops.NewForHost(svcCtx, req.HostID)
		opts := containerops.LogsStreamOptions{
			Tail:       req.Tail,
			Since:      req.Since,
			Timestamps: req.Timestamps,
			Follow:     req.Follow,
			Search:     req.Search,
		}

		// 每收到一行就以 SSE "log" 事件下发；ctx 取消（客户端断连）时终止。
		err := svc.LogsStream(ctx, req.Id, opts, func(line string) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}
			sendEvent("log", line)
			return true
		})

		if err != nil && ctx.Err() == nil {
			// 非客户端主动断开的错误，作为 SSE "error" 事件告知前端
			logx.Errorf("流式日志读取失败 container=%s: %v", req.Id, err)
			sendEvent("error", "读取日志失败: "+err.Error())
			return
		}
		// 非跟随模式：读完全部日志后发送 "end" 事件，前端据此停止 loading 并关闭连接
		if !req.Follow && ctx.Err() == nil {
			sendEvent("end", "")
		}
	}
}
