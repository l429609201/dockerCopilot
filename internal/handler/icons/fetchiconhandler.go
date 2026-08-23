package icons

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	faviconLogic "github.com/l429609201/dockerCopilot/internal/logic/favicon"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// persistImagesDir 持久化图标存放目录（对应静态路由 /images/）。
const persistImagesDir = "/data/images"

// fetchIconReq 自动抓取并持久化图标的请求体。
type fetchIconReq struct {
	ImageName string `json:"imageName"`
	URL       string `json:"url"` // 容器访问地址，如 http://192.168.1.2:8080
}

// FetchIconHandler 抓取站点 favicon、下载图片持久化到 /data/images，并绑定到镜像名。
// 与仅存外链不同：图片本体落盘，外链失效或跨设备也能显示。
func FetchIconHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req fetchIconReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeUploadError(w, http.StatusBadRequest, "请求体解析失败")
			return
		}
		if err := validateImageName(req.ImageName); err != nil {
			writeUploadError(w, http.StatusBadRequest, err.Error())
			return
		}
		// 1. 解析出可用 favicon 外链
		iconURL, err := faviconLogic.Resolve(r.Context(), req.URL)
		if err != nil {
			writeUploadError(w, http.StatusOK, "未找到可用图标: "+err.Error())
			return
		}
		// 2. 下载图片并持久化落盘
		localPath, err := downloadAndPersist(iconURL)
		if err != nil {
			writeUploadError(w, http.StatusOK, "下载图标失败: "+err.Error())
			return
		}
		// 3. 写入 imageLogos.js 映射
		if _, e := os.Stat(imageLogosPath); os.IsNotExist(e) {
			_ = os.MkdirAll(imageUploadDir, 0o755)
			_ = os.WriteFile(imageLogosPath, []byte("export const customImageLogos = {\n}\n"), 0644)
		}
		if err := writeImageLogoValue(imageLogosPath, req.ImageName, localPath); err != nil {
			writeUploadError(w, http.StatusInternalServerError, "写入配置失败: "+err.Error())
			return
		}
		logx.Infof("图标持久化成功: %s -> %s (源 %s)", req.ImageName, localPath, iconURL)
		httpx.OkJsonCtx(r.Context(), w, types.Resp{Code: 200, Msg: "Success", Data: localPath})
	}
}

// downloadAndPersist 下载图片到 /data/images，返回可供前端访问的 /images/<file> 路径。
// 文件名用 URL 的 sha1 前 16 位 + 扩展名，天然去重（同图标只存一份）。
func downloadAndPersist(iconURL string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(iconURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("状态码 %d", resp.StatusCode)
	}
	// 限制 5MB，避免异常大文件
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("图标内容为空")
	}
	ext := pickIconExt(iconURL, resp.Header.Get("Content-Type"))
	sum := sha1.Sum([]byte(iconURL))
	filename := fmt.Sprintf("%x%s", sum[:8], ext)
	if err := os.MkdirAll(persistImagesDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(persistImagesDir, filename), data, 0644); err != nil {
		return "", err
	}
	return "/images/" + filename, nil
}

// pickIconExt 从 URL 或 Content-Type 推断图片扩展名，默认 .png。
func pickIconExt(iconURL, contentType string) string {
	if mt, _, e := mime.ParseMediaType(contentType); e == nil {
		switch mt {
		case "image/png":
			return ".png"
		case "image/jpeg":
			return ".jpg"
		case "image/webp":
			return ".webp"
		case "image/gif":
			return ".gif"
		case "image/svg+xml":
			return ".svg"
		case "image/x-icon", "image/vnd.microsoft.icon":
			return ".ico"
		}
	}
	lower := strings.ToLower(iconURL)
	for _, e := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".ico"} {
		if strings.Contains(lower, e) {
			return e
		}
	}
	return ".png"
}