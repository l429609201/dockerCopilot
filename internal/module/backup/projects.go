package backup

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	composemod "github.com/l429609201/dockerCopilot/internal/module/compose"
	"github.com/l429609201/dockerCopilot/internal/types"
)

func consumePull(r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		var event struct {
			Error string `json:"error"`
		}
		err := dec.Decode(&event)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if event.Error != "" {
			return fmt.Errorf("拉取备份镜像失败: %s", event.Error)
		}
	}
}

func (s *Service) scanConfig() ([]string, int) {
	paths := s.svcCtx.Config.Compose.ScanPaths
	depth := s.svcCtx.Config.Compose.MaxDepth
	if s.svcCtx.AppConfig != nil {
		dyn := s.svcCtx.AppConfig.Get().Compose
		if len(dyn.ScanPaths) > 0 {
			paths = dyn.ScanPaths
		}
		if dyn.MaxDepth > 0 {
			depth = dyn.MaxDepth
		}
	}
	return paths, depth
}

func (s *Service) projects() []types.BackupProject {
	paths, depth := s.scanConfig()
	out := []types.BackupProject{}
	for _, p := range composemod.NewScanner(paths, depth).Scan() {
		if _, _, err := composemod.SafeResolveDir(paths, p.Dir); err == nil {
			out = append(out, types.BackupProject{ID: p.ID, Name: p.Name, Dir: p.Dir, Files: p.Files})
		}
	}
	return out
}

// localFile 对原始文件和真实扫描根目录再次校验，禁止符号链接逃逸与特殊文件。
func (s *Service) localFile(ctx context.Context, tw *tar.Writer, p types.BackupProject, name, prefix string, budget *int64) error {
	paths, _ := s.scanConfig()
	dir, root, err := composemod.SafeResolveDir(paths, p.Dir)
	if err != nil {
		return err
	}
	full, err := composemod.SafeResolveFile(dir, name)
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("Compose 文件超出扫描根目录")
	}
	if err = safeSource(real); err != nil {
		return err
	}
	f, err := openRegular(real)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 10*1024*1024 || info.Size() > *budget {
		return fmt.Errorf("Compose 文件过大")
	}
	*budget -= info.Size()
	if err = ctx.Err(); err != nil {
		return err
	}
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	h.Name = prefix + "/" + name
	h.Mode = 0600
	if err = tw.WriteHeader(h); err != nil {
		return err
	}
	_, err = io.CopyN(tw, f, info.Size())
	return err
}
