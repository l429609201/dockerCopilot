package ops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/golang-jwt/jwt"
	"github.com/gorilla/websocket"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// upgrader 将 HTTP 升级为 WebSocket。同源策略此处放宽（已由 JWT 鉴权保护）。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// resizeMsg 前端发来的终端尺寸调整消息（type=resize）。
type resizeMsg struct {
	Type string `json:"type"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// ExecWSHandler 建立容器交互式终端：升级 WebSocket，创建 Tty 的 Docker Exec，
// 双向转发 stdin/stdout，实现类似 Portainer 的控制台。
func ExecWSHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r)
		if id == "" {
			http.Error(w, "缺少容器ID", http.StatusBadRequest)
			return
		}
		// WebSocket 无法带 Authorization 头，改从 query token 校验 JWT
		if !validWSToken(r, svcCtx.Config.Auth.AccessSecret) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		// 命令与用户来自 query（?cmd=/bin/bash&user=root）
		cmd := r.URL.Query().Get("cmd")
		if cmd == "" {
			cmd = "/bin/sh"
		}
		user := r.URL.Query().Get("user")

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logx.Errorf("WebSocket 升级失败: %v", err)
			return
		}
		defer conn.Close()

		ctx := context.Background()
		execCfg := container.ExecOptions{
			User:         user,
			Tty:          true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          strings.Fields(cmd),
		}
		created, err := svcCtx.DockerClient.ContainerExecCreate(ctx, id, execCfg)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("创建 exec 失败: "+err.Error()))
			return
		}
		hijack, err := svcCtx.DockerClient.ContainerExecAttach(ctx, created.ID, container.ExecStartOptions{Tty: true})
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("附加 exec 失败: "+err.Error()))
			return
		}
		defer hijack.Close()

		pumpExec(conn, hijack, svcCtx, created.ID)
	}
}

// pumpExec 在 WebSocket 与容器 exec 之间双向转发数据，并处理 resize 控制消息。
// hijack.Reader 读容器输出，hijack.Conn 写容器输入。
func pumpExec(conn *websocket.Conn, hijack dockertypes.HijackedResponse, svcCtx *svc.ServiceContext, execID string) {
	done := make(chan struct{})

	// 容器输出 -> 浏览器
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := hijack.Reader.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					logx.Errorf("读取容器输出失败: %v", err)
				}
				return
			}
		}
	}()

	// 浏览器输入 -> 容器；文本消息若为 resize JSON 则调整终端尺寸
	for {
		select {
		case <-done:
			return
		default:
		}
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.TextMessage {
			var rm resizeMsg
			if json.Unmarshal(data, &rm) == nil && rm.Type == "resize" {
				_ = svcCtx.DockerClient.ContainerExecResize(context.Background(), execID,
					container.ResizeOptions{Height: rm.Rows, Width: rm.Cols})
				continue
			}
		}
		if _, werr := hijack.Conn.Write(data); werr != nil {
			return
		}
	}
}

// pathID 从 go-zero 注入的 path 参数中提取 :id。
func pathID(r *http.Request) string {
	vars := pathvar.Vars(r)
	return vars["id"]
}

// validWSToken 校验 query 中的 token（HS256，密钥为 AccessSecret）。
// 支持 ?token=xxx，兼容前端把 JWT 放到 query 传递的方式。
func validWSToken(r *http.Request, secret string) bool {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		// 兼容 Sec-WebSocket-Protocol 或 Authorization 头（部分客户端可设）
		auth := r.Header.Get("Authorization")
		tokenStr = strings.TrimPrefix(auth, "Bearer ")
	}
	if tokenStr == "" {
		return false
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errUnexpectedSigning
		}
		return []byte(secret), nil
	})
	return err == nil && token.Valid
}

var errUnexpectedSigning = errorString("unexpected signing method")

type errorString string

func (e errorString) Error() string { return string(e) }
