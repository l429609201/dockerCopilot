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
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var imageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*$`)

var allowedImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

func UploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析 Multipart 表单
		err := r.ParseMultipartForm(10 << 20) // 10MB 限制
		if err != nil {
			writeUploadError(w, http.StatusBadRequest, "failed to parse form")
			return
		}

		// 2. 获取文件和 Key
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

		// 3. 确保目录存在 (防御性编程)
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

		// 5. 更新 imageLogos.js
		jsPath := imageLogosPath
		if err := updateImageLogosJS(jsPath, imageNameKey, filename); err != nil {
			_ = os.Remove(dstPath)
			writeUploadError(w, http.StatusInternalServerError, "failed to update config")
			return
		}

		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "Success",
			Data: filename,
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
		return "", fmt.Errorf("only png, jpg, jpeg, webp and gif files are allowed")
	}

	detectedType := http.DetectContentType(header[:n])
	if detectedType != expectedType {
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

func updateImageLogosJS(filePath, imageName, filename string) error {
	// 读取文件
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	// 前端使用的容器路径
	containerPath := fmt.Sprintf("/src/config/image/%s", filename)

	if strings.Contains(content, fmt.Sprintf(`"%s"`, imageName)) {
		// 更新现有行
		re := regexp.MustCompile(fmt.Sprintf(`"%s"\s*:\s*".*"`, regexp.QuoteMeta(imageName)))
		content = re.ReplaceAllString(content, fmt.Sprintf(`"%s": "%s"`, imageName, containerPath))
	} else {
		// 插入新行
		// 查找 `export const customImageLogos = {`
		startIdx := strings.Index(content, "export const customImageLogos = {")
		if startIdx == -1 {
			return fmt.Errorf("invalid config format")
		}
		// 尝试查找右大括号。这里假设它是最后一个右大括号逻辑或者是文件末尾。
		// 一个简单的启发式方法：插入到最后一个 `}` 或 `};` 之前。
		lastBraceIdx := strings.LastIndex(content, "}")
		if lastBraceIdx == -1 || lastBraceIdx < startIdx {
			return fmt.Errorf("invalid config format, no closing brace")
		}

		newLine := fmt.Sprintf(`  "%s": "%s",`, imageName, containerPath)
		// 插入到最后一个大括号之前
		content = content[:lastBraceIdx] + newLine + "\n" + content[lastBraceIdx:]
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}
