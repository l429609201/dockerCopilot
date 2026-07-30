package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
)

func TestDelRestoreRejectsTraversalFilename(t *testing.T) {
	parentDir := t.TempDir()
	backupDir := filepath.Join(parentDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}
	t.Setenv("BACKUP_DIR", backupDir)

	outsideFile := filepath.Join(parentDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	logic := NewDelRestoreLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.DelRestore(&types.DelContainerBackupReq{
		Filename: "../outside.txt",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Code != 400 {
		t.Fatalf("expected 400 for traversal filename, got %d", resp.Code)
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("expected outside file to remain, stat error: %v", err)
	}
}
