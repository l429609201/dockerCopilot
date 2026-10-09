package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
)

var errOptionalMissing = errors.New("可选环境文件不存在")

// ensureHelperImage 仅在目标 daemon 缺少镜像时拉取，并检查拉取流中的错误。
func ensureHelperImage(ctx context.Context, c *client.Client) error {
	_, _, err := c.ImageInspectWithRaw(ctx, helperImage)
	if err == nil {
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return err
	}
	stream, err := c.ImagePull(ctx, helperImage, image.PullOptions{})
	if err != nil {
		return err
	}
	defer stream.Close()
	return consumePull(stream)
}

// withArchiveHelper 在目标 daemon 创建只读 helper，绝不把远程来源当成本地路径。
func withArchiveHelper(ctx context.Context, c *client.Client, m mount.Mount, sourcePath string, fileOnly bool, fn func(io.Reader) error) (err error) {
	if m.Type == mount.TypeVolume {
		if _, err = c.VolumeInspect(ctx, m.Source); err != nil {
			return fmt.Errorf("卷 %s 不存在或不可读: %w", m.Source, err)
		}
		m.VolumeOptions = &mount.VolumeOptions{NoCopy: true}
	}
	m.Target = "/input"
	m.ReadOnly = true
	// 仅恢复跨 UID 目录读取所需能力，只读挂载仍禁止写入，且禁止其它能力、网络和提权。
	created, err := c.ContainerCreate(ctx, &container.Config{
		Image: helperImage,
		Cmd: []string{"sleep", "3600"},
		NetworkDisabled: true,
		Labels: map[string]string{"dockercopilot.backup.helper": "true"},
	}, &container.HostConfig{
		Mounts: []mount.Mount{m},
		NetworkMode: "none",
		ReadonlyRootfs: true,
		CapDrop: []string{"ALL"},
		CapAdd: []string{"DAC_OVERRIDE"},
		SecurityOpt: []string{"no-new-privileges:true"},
		Resources: container.Resources{Memory: 128 * 1024 * 1024, PidsLimit: int64Ptr(32)},
	}, nil, nil, "dc-backup-"+uuid.New().String())
	if err != nil {
		return fmt.Errorf("创建只读 helper 失败: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := c.ContainerRemove(cleanup, created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true}); e != nil {
			// 清理失败不得被可选文件缺失的 sentinel 掩盖。
			err = fmt.Errorf("%v；清理 helper %s 失败: %w", err, created.ID, e)
		}
	}()
	if err = c.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return err
	}
	execCmd := []string{
		"sh", "-c",
		"bad=$(find \"$1\" ! -type f ! -type d -print -quit) || exit 44; test -z \"$bad\" || { echo '挂载包含特殊文件或链接' >&2; exit 43; }; exec tar -c -f - -C \"$2\" -- \"$3\"",
		"backup", sourcePath, path.Dir(sourcePath), path.Base(sourcePath),
	}
	// 文件参数通过独立 argv 传递，脚本不拼接用户路径；可选文件不存在采用独立退出码。
	if fileOnly {
		execCmd = []string{
			"sh", "-c",
			"test -e \"$1\" || exit 42; test -f \"$1\" && test ! -L \"$1\" || exit 43; exec tar -c -f - -C \"$2\" -- \"$3\"",
			"backup", sourcePath, path.Dir(sourcePath), path.Base(sourcePath),
		}
	}
	exec, err := c.ContainerExecCreate(ctx, created.ID, container.ExecOptions{
		Cmd: execCmd, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return err
	}
	attach, err := c.ContainerExecAttach(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return err
	}
	defer attach.Close()
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	stderr := &boundedBuffer{limit: 64 * 1024}
	go func() {
		_, e := stdcopy.StdCopy(pw, stderr, attach.Reader)
		_ = pw.CloseWithError(e)
		done <- e
	}()
	// 取消关闭 hijack，消费失败关闭管道解除反压；成功必须读完 tar 的尾部填充。
	stop := context.AfterFunc(ctx, func() {
		attach.Close()
		_ = pr.CloseWithError(ctx.Err())
	})
	defer stop()
	err = fn(pr)
	if err == nil {
		_, err = io.Copy(io.Discard, pr)
	}
	_ = pr.Close()
	attach.Close()
	streamErr := <-done
	if err != nil {
		return err
	}
	if streamErr != nil {
		return streamErr
	}
	for {
		state, e := c.ContainerExecInspect(ctx, exec.ID)
		if e != nil {
			return e
		}
		if !state.Running {
			if state.ExitCode == 42 {
				return errOptionalMissing
			}
			if state.ExitCode != 0 {
				return fmt.Errorf("目标主机 tar 失败或文件在备份期间变化，退出码 %d: %s", state.ExitCode, strings.TrimSpace(stderr.String()))
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func int64Ptr(v int64) *int64 { return &v }

func joinError(a, b error) error {
	if a == nil {
		return b
	}
	return fmt.Errorf("%v；%w", a, b)
}

// copyArchive 不解包到本地，只保留安全普通文件和目录，拒绝链接、设备等特殊项。
func copyArchive(ctx context.Context, tw *tar.Writer, reader io.Reader, prefix string, budget *int64) error {
	tr := tar.NewReader(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := path.Clean(h.Name)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(h.Name, "\x00\r\n\\") {
			return fmt.Errorf("归档包含不安全路径")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeDir {
			return fmt.Errorf("拒绝特殊文件或链接: %s", h.Name)
		}
		if h.Size < 0 || h.Size > *budget {
			return fmt.Errorf("备份内容超过 10GiB 上限")
		}
		*budget -= h.Size
		h.Name = path.Join(prefix, clean)
		h.Linkname = ""
		h.PAXRecords = nil
		h.Xattrs = nil
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		if h.Typeflag != tar.TypeDir {
			if _, err = io.CopyN(tw, tr, h.Size); err != nil {
				return err
			}
		}
	}
}
