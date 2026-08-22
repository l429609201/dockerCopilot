package favicon

import (
	"net/http"
	"strings"

	faviconLogic "github.com/onlyLTY/dockerCopilot/internal/logic/favicon"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ResolveHandler 解析指定站点的 favicon 地址。
// 请求：GET /api/favicon/resolve?url=http://ip:port
// 始终返回 200，未找到时 data.url 为空，避免前端因 4xx 报错。
func ResolveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := strings.TrimSpace(r.URL.Query().Get("url"))
		iconURL, err := faviconLogic.Resolve(r.Context(), target)
		if err != nil {
			httpx.OkJsonCtx(r.Context(), w, types.Resp{
				Code: 200, Msg: err.Error(), Data: map[string]interface{}{"url": ""},
			})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200, Msg: "success", Data: map[string]interface{}{"url": iconURL},
		})
	}
}
