package container

import (
	"context"
	"testing"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
)

func TestRestoreRejectsTraversalFilename(t *testing.T) {
	logic := NewRestoreLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.Restore(&types.ContainerRestoreReq{
		Filename: "../config/image/evil.json",
	})
	if err == nil {
		t.Fatal("expected error for traversal filename")
	}
	if resp.Code != 400 {
		t.Fatalf("expected 400 for traversal filename, got %d", resp.Code)
	}
}
