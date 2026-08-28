package icons

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var imageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*$`)

var allowedImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
}

// UploadRequest 上传图标请求
type UploadRequest struct {
	ImageName  string `json:"imageName"`  // 镜像名或容器名
	TargetType string `json:"targetType"` // "container" 或 "image"，默认 "image"
}

func UploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析 Multipart 表单
		err := r.ParseMultipartForm(10 << 20) // 10MB 限制
		if err != nil {
			writeUploadError(w, http.StatusBadRequest, "failed to parse form")
			return
		}

		// 2. 获取文件和参数
		file, handler, err := r.FormFile("file")
		if err != nil {
			writeUploadError(w, http.StatusBadRequest, "failed to get file")
			return
		}
		defer file.Close()

		imageNameKey := r.FormValue("imageName")
		if err := validateImageName(imageNameKey); err != nil {
			writeUploadError(w, http.StatusBadRequest, err.Error())
			return
		}

		// 获取目标类型，默认为 "image"
		targetType := r.FormValue("targetType")
		if targetType == "" {
			targetType = "image"
		}
		if targetType != "container" && targetType != "image" {
			writeUploadError(w, http.StatusBadRequest, "targetType must be 'container' or 'image'")
			return
		}

		// 3. 确保目录存在（统一使用绝对路径常量，与抓取/静态服务保持一致）
		dataPath := imageUploadDir
		if err := os.MkdirAll(dataPath, 0o755); err != nil {
			writeUploadError(w, http.StatusInternalServerError, "failed to prepare upload dir")
			return
		}

		// 4. 确定文件名
		filename, err := generateStoredFilename(file, handler)
		if err != nil {
			writeUploadError(w, http.StatusBadRequest, err.Error())
			return
		}

		dstPath := filepath.Join(dataPath, filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			writeUploadError(w, http.StatusInternalServerError, "failed to create file on server")
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			writeUploadError(w, http.StatusInternalServerError, "failed to copy file content")
			return
		}

		// 5. 更新图标配置（新格式）
		iconURL := fmt.Sprintf("/images/%s", filename)
		priority := 2 // 镜像级
		if targetType == "container" {
			priority = 1 // 容器级
		}

		if err := addOrUpdateIcon(imageNameKey, targetType, iconURL, priority); err != nil {
			_ = os.Remove(dstPath)
			writeUploadError(w, http.StatusInternalServerError, "failed to update config")
			return
		}

		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "Success",
			Data: map[string]interface{}{
				"filename": filename,
				"iconUrl":  iconURL,
			},
		})
	}
}

func validateImageName(imageName string) error {
	if imageName == "" {
		return fmt.Errorf("imageName is required")
	}
	if !imageNamePattern.MatchString(imageName) {
		return fmt.Errorf("invalid imageName")
	}
	return nil
}

func generateStoredFilename(file multipart.File, handler *multipart.FileHeader) (string, error) {
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to inspect upload")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to reset upload stream")
	}

	ext := strings.ToLower(filepath.Ext(handler.Filename))
	expectedType, ok := allowedImageTypes[ext]
	if !ok {
		return "", fmt.Errorf("only png, jpg, jpeg, webp, gif, ico, svg files are allowed")
	}

	detectedType := http.DetectContentType(header[:n])
	// SVG 和 ICO 文件的 MIME 类型检测不准确，跳过检测
	if ext != ".svg" && ext != ".ico" && detectedType != expectedType {
		return "", fmt.Errorf("uploaded file content does not match its extension")
	}

	return uuid.NewString() + ext, nil
}

func writeUploadError(w http.ResponseWriter, statusCode int, msg string) {
	httpx.WriteJson(w, statusCode, types.Resp{
		Code: statusCode,
		Msg:  msg,
		Data: map[string]interface{}{},
	})
}
