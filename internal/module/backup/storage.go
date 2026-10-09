package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/types"
)

var storageMu sync.Mutex

func backupDir() string {
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		dir = "/data/backups"
	}
	return filepath.Clean(dir)
}

// paths 仅接受规范 UUID，完成标记不使用 .json，避免混入旧配置快照列表。
func paths(id string) (string, string, error) {
	u, err := uuid.Parse(id)
	if err != nil || u.String() != id {
		return "", "", fmt.Errorf("非法备份 ID")
	}
	dir := backupDir()
	return filepath.Join(dir, id+".tar.gz"), filepath.Join(dir, id+".manifest"), nil
}

// openRegular 在打开时拒绝符号链接，避免检查与打开之间的路径替换。
func openRegular(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("备份文件不是普通文件")
	}
	return file, nil
}

func loadManifest(id string) (*types.BackupManifest, error) {
	archivePath, manifestPath, err := paths(id)
	if err != nil {
		return nil, err
	}
	file, err := openRegular(manifestPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var manifest types.BackupManifest
	if err = json.NewDecoder(io.LimitReader(file, 1024*1024)).Decode(&manifest); err != nil {
		return nil, err
	}
	if manifest.ID != id {
		return nil, fmt.Errorf("备份 manifest ID 不匹配")
	}
	archive, err := openRegular(archivePath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	info, err := archive.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != manifest.Size {
		return nil, fmt.Errorf("备份大小与 manifest 不匹配")
	}
	return &manifest, nil
}

func (s *Service) List() ([]types.BackupManifest, error) {
	storageMu.Lock()
	defer storageMu.Unlock()
	out := []types.BackupManifest{}
	entries, err := os.ReadDir(backupDir())
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".manifest") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".manifest")
		manifest, err := loadManifest(id)
		if err != nil {
			continue
		}
		out = append(out, *manifest)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (s *Service) Open(id string) (*os.File, *types.BackupManifest, error) {
	storageMu.Lock()
	defer storageMu.Unlock()
	manifest, err := loadManifest(id)
	if err != nil {
		return nil, nil, err
	}
	archivePath, _, _ := paths(id)
	file, err := openRegular(archivePath)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || info.Size() != manifest.Size {
		_ = file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("备份大小与 manifest 不匹配")
	}
	return file, manifest, nil
}

func (s *Service) Delete(id string) error {
	storageMu.Lock()
	defer storageMu.Unlock()
	if _, err := loadManifest(id); err != nil {
		return err
	}
	archivePath, manifestPath, _ := paths(id)
	// 先移除完成标记，任何后续删除失败都不会继续发布可下载档案。
	if err := os.Remove(manifestPath); err != nil {
		return err
	}
	return os.Remove(archivePath)
}
