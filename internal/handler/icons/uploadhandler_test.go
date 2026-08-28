package icons

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/l429609201/dockerCopilot/internal/svc"
)

func TestUploadHandlerRejectsNonImageFiles(t *testing.T) {
	tempDir := t.TempDir()
	imageDir := filepath.Join(tempDir, "image")
	testFilename := "codex-upload-vuln.json"

	// 将上传目录重定向到临时目录，避免污染真实 /data/images
	originalImageUploadDir := imageUploadDir
	imageUploadDir = imageDir
	t.Cleanup(func() {
		imageUploadDir = originalImageUploadDir
		_ = os.Remove(filepath.Join(imageDir, testFilename))
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", testFilename)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := io.WriteString(fileWriter, `{"not":"an image"}`); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.WriteField("imageName", "nginx"); err != nil {
		t.Fatalf("failed to write imageName: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/icons", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	UploadHandler(&svc.ServiceContext{})(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-image upload, got %d with body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(imageDir, testFilename)); !os.IsNotExist(err) {
		t.Fatalf("expected non-image upload to be rejected without writing file, stat err=%v", err)
	}
}
