package icons

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// setIconURLReq 通过 URL 绑定图标的请求体。
type setIconURLReq struct {
	ImageName string `json:"imageName"`
	URL       string `json:"url"`
}

// SetIconURLHandler 直接用一个图标 URL 绑定到指定镜像名。
// 相比上传图片，URL 方式更快，且在生产环境（本地静态目录不可访问时）更可靠。
func SetIconURLHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setIconURLReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeUploadError(w, http.StatusBadRequest, "请求体解析失败")
			return
		}
		if err := validateImageName(req.ImageName); err != nil {
			writeUploadError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateIconURL(req.URL); err != nil {
			writeUploadError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := os.Stat(imageLogosPath); os.IsNotExist(err) {
			_ = os.MkdirAll(imageUploadDir, 0o755)
			_ = os.WriteFile(imageLogosPath, []byte("export const customImageLogos = {\n}\n"), 0644)
		}
		if err := writeImageLogoValue(imageLogosPath, req.ImageName, req.URL); err != nil {
			writeUploadError(w, http.StatusInternalServerError, "写入配置失败: "+err.Error())
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "Success",
			Data: req.URL,
		})
	}
}

// validateIconURL 校验图标 URL：必须是 http/https 绝对地址，且不含引号（防注入 js 文件）。
func validateIconURL(u string) error {
	if u == "" {
		return errIcon("url is required")
	}
	if strings.ContainsAny(u, "\"'\n\r") {
		return errIcon("url 包含非法字符")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return errIcon("url 格式错误")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errIcon("url 必须以 http:// 或 https:// 开头")
	}
	if parsed.Host == "" {
		return errIcon("url 缺少主机名")
	}
	return nil
}

type iconErr string

func (e iconErr) Error() string { return string(e) }
func errIcon(s string) error    { return iconErr(s) }