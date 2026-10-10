package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

type plan struct {
	containers []dockertypes.ContainerJSON
	projects []types.BackupProject
	mounts []types.BackupMount
	req types.BackupCreateReq
	cli *client.Client
}

func overlaps(a, b string) bool {
	return a == "/" || b == "/" || a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// prepare 在停止任何容器前检查选择、共享写入者及自备份递归。
func (s *Service) prepare(ctx context.Context, req types.BackupCreateReq) (*plan, error) {
	req.HostID = hostID(req.HostID)
	if len(req.ContainerIDs) == 0 && len(req.ProjectIDs) == 0 {
		return nil, fmt.Errorf("请选择容器或 Compose 项目")
	}
	if len(req.ContainerIDs) > 100 || len(req.ProjectIDs) > 100 || len(req.MountKeys) > 1000 {
		return nil, fmt.Errorf("选择数量超过上限")
	}
	if len(req.ProjectIDs) > 0 && (!req.IncludeCompose || req.HostID != appconfig.DockerHostLocalID) {
		return nil, fmt.Errorf("扫描项目仅可在本地主机备份原始 Compose 文件")
	}
	c, err := s.client(req.HostID)
	if err != nil {
		return nil, err
	}
	p := &plan{req: req, cli: c}
	selected := map[string]bool{}
	available := map[string]types.BackupMount{}
	for _, id := range req.ContainerIDs {
		in, err := c.ContainerInspect(ctx, id)
		if err != nil {
			return nil, err
		}
		if selected[in.ID] {
			continue
		}
		selected[in.ID] = true
		if in.Config != nil && in.Config.Labels["dockercopilot.backup.helper"] == "true" {
			return nil, fmt.Errorf("不能备份临时 helper")
		}
		if req.StopContainers && in.State != nil && in.State.Paused {
			return nil, fmt.Errorf("容器 %s 处于暂停状态，首版停机备份不改变暂停状态，请先显式解除暂停", in.Name)
		}
		if req.StopContainers && req.HostID == appconfig.DockerHostLocalID && utiles.IsSelfContainer(s.svcCtx, in.ID) {
			return nil, fmt.Errorf("不能停止 DockerCopilot 自身进行备份")
		}
		p.containers = append(p.containers, in)
		for _, m := range describeContainer(in).Mounts {
			if m.Allowed {
				available[mountKey(m)] = m
			}
		}
	}
	if req.IncludeData {
		if req.MountKeys == nil {
			for _, m := range available {
				p.mounts = append(p.mounts, m)
			}
		} else {
			seen := map[string]bool{}
			for _, key := range req.MountKeys {
				m, ok := available[key]
				if !ok {
					return nil, fmt.Errorf("挂载未选择或不允许备份: %s", key)
				}
				if !seen[key] {
					p.mounts = append(p.mounts, m)
					seen[key] = true
				}
			}
		}
	}
	sort.Slice(p.mounts, func(i, j int) bool { return mountKey(p.mounts[i]) < mountKey(p.mounts[j]) })
	if err = s.checkOutputOverlap(ctx, p); err != nil {
		return nil, err
	}
	if req.StopContainers && len(p.mounts) > 0 {
		if err = checkSharedRunning(ctx, p, selected); err != nil {
			return nil, err
		}
	}
	projects := map[string]types.BackupProject{}
	if len(req.ProjectIDs) > 0 {
		for _, project := range s.projects() {
			projects[project.ID] = project
		}
	}
	seenProjects := map[string]bool{}
	for _, id := range req.ProjectIDs {
		project, ok := projects[id]
		if !ok {
			return nil, fmt.Errorf("Compose 项目不在当前扫描范围内")
		}
		if !seenProjects[id] {
			p.projects = append(p.projects, project)
			seenProjects[id] = true
		}
	}
	if len(p.mounts) == 0 && len(p.projects) == 0 && !(req.IncludeCompose && p.hasComposeCandidate()) {
		return nil, fmt.Errorf("所选内容没有可备份的数据挂载或 Compose 原文件")
	}
	return p, nil
}

func (p *plan) hasComposeCandidate() bool {
	for _, in := range p.containers {
		if describeContainer(in).Compose.Candidate {
			return true
		}
	}
	return false
}

// checkOutputOverlap 利用自身 inspect 推导 BACKUP_DIR 的 daemon 主机来源。
func (s *Service) checkOutputOverlap(ctx context.Context, p *plan) error {
	if p.req.HostID != appconfig.DockerHostLocalID {
		return nil
	}
	abs, err := filepath.Abs(backupDir())
	if err != nil {
		return err
	}
	outputSources := []string{abs}
	self := utiles.GetSelfContainerID()
	if self != "" {
		in, inspectErr := p.cli.ContainerInspect(ctx, self)
		if inspectErr == nil {
			for _, m := range in.Mounts {
				if m.Destination == "" || !(abs == m.Destination || strings.HasPrefix(abs, m.Destination+"/")) {
					continue
				}
				rel := strings.TrimPrefix(strings.TrimPrefix(abs, m.Destination), "/")
				outputSources = append(outputSources, path.Join(m.Source, rel))
				if m.Type == mount.TypeVolume {
					for _, chosen := range p.mounts {
						if chosen.Type == "volume" && chosen.Name == m.Name {
							return fmt.Errorf("不能备份包含输出目录的自身卷")
						}
					}
				}
			}
		}
	}
	for _, m := range p.mounts {
		for _, out := range outputSources {
			if m.Source != "" && overlaps(m.Source, out) {
				return fmt.Errorf("挂载与备份输出目录重叠，拒绝递归备份: %s", m.Source)
			}
		}
	}
	return nil
}

// checkSharedRunning 不仅比较卷名，还比较 bind 与 volume 实际来源的父子目录交集。
func checkSharedRunning(ctx context.Context, p *plan, selected map[string]bool) error {
	all, err := p.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}
	for _, item := range all {
		if selected[item.ID] {
			continue
		}
		in, err := p.cli.ContainerInspect(ctx, item.ID)
		if err != nil {
			return err
		}
		if in.State == nil || !in.State.Running {
			continue
		}
		for _, other := range in.Mounts {
			for _, chosen := range p.mounts {
				sameVolume := chosen.Type == "volume" && other.Type == mount.TypeVolume && chosen.Name == other.Name
				if sameVolume || (other.Source != "" && chosen.Source != "" && overlaps(chosen.Source, other.Source)) {
					return fmt.Errorf("运行容器 %s 共享所选数据挂载，请同时选择该容器", strings.TrimPrefix(in.Name, "/"))
				}
			}
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, req types.BackupCreateReq) (string, error) {
	p, err := s.prepare(ctx, req)
	if err != nil {
		return "", err
	}
	id := uuid.New().String()
	s.progress(id, 0, "等待备份任务执行", false, false, false)
	err = s.svcCtx.TaskManager.TryStart(id, "backup:"+p.req.HostID, "backup", func(taskCtx context.Context) {
		taskCtx, cancel := context.WithTimeout(taskCtx, 55*time.Minute)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				s.progress(id, 100, fmt.Sprintf("备份异常: %v", recovered), true, true, false)
			}
		}()
		// 排队期间容器或挂载可能改变，进入任务时重新验证。
		fresh, e := s.prepare(taskCtx, p.req)
		if e == nil {
			e = s.execute(taskCtx, id, fresh)
		}
		if e != nil {
			s.progress(id, 100, "备份失败: "+e.Error(), true, true, taskCtx.Err() != nil)
			return
		}
		s.progress(id, 100, "备份完成；备份 ID: "+id, true, false, false)
	})
	if err != nil {
		s.progress(id, 100, err.Error(), true, true, false)
		return "", err
	}
	return id, nil
}

func (s *Service) progress(id string, percentage int, msg string, done, failed, canceled bool) {
	s.svcCtx.UpdateProgress(id, svc.TaskProgress{
		TaskID: id, Name: "实质备份", TaskType: "backup", ResourceID: id,
		Percentage: percentage, Message: msg, DetailMsg: msg,
		IsDone: done, Failed: failed, Canceled: canceled,
	})
}

type limitedWriter struct {
	w io.Writer
	n int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > MaxArchiveBytes-w.n {
		return 0, fmt.Errorf("压缩包超过 10GiB 上限")
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

func (s *Service) execute(ctx context.Context, id string, p *plan) (err error) {
	m := types.BackupManifest{
		ID: id, HostID: p.req.HostID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Items: []types.BackupItem{},
		Warnings: []string{
			"首版不自动恢复；仅保留普通文件和目录权限，不支持特殊文件、链接、ACL 和扩展属性；原始数据与压缩包各限 10GiB",
			"挂载只从已选容器的 Docker inspect 来源读取；目录别名和容器外部写入者无法完全识别，不保证应用事务一致性",
		},
	}
	if p.req.IncludeCompose {
		m.Warnings = append(m.Warnings, "Compose 仅归档原始配置文件及可选 .env；include、env_file、secrets、configs 等外部依赖需另行选择数据挂载备份")
	}
	if p.req.StopContainers {
		m.Warnings = append(m.Warnings, "停机备份期间请勿重启 DockerCopilot；进程崩溃无法由内存取消句柄自动恢复容器")
	}
	if !p.req.StopContainers {
		m.Warnings = append(m.Warnings, "在线备份不保证数据库事务一致性")
	}
	if !p.req.IncludeEnv {
		m.Warnings = append(m.Warnings, "未包含容器运行时环境变量，手动恢复时需另行配置")
	} else {
		m.Warnings = append(m.Warnings, "归档包含环境配置，可能含密码和令牌，请妥善保管")
	}
	s.progress(id, 5, "准备只读备份 helper", false, false, false)
	if len(p.mounts) > 0 || (p.req.IncludeCompose && p.hasComposeCandidate()) {
		if err = ensureHelperImage(ctx, p.cli); err != nil {
			return err
		}
	}
	stopped := []string{}
	restored := false
	restore := func() error {
		var result error
		for _, cid := range stopped {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			e := p.cli.ContainerStart(restoreCtx, cid, container.StartOptions{})
			cancel()
			if e != nil {
				result = joinError(result, fmt.Errorf("恢复容器 %s 原运行状态失败: %w", cid, e))
			}
		}
		return result
	}
	defer func() {
		if !restored {
			err = joinError(err, restore())
		}
	}()
	if p.req.StopContainers {
		for _, in := range p.containers {
			if in.State == nil || !in.State.Running {
				continue
			}
			// 先登记以覆盖 Docker Stop 返回超时但实际上已经停止的情况。
			stopped = append(stopped, in.ID)
			timeout := 30
			if err = p.cli.ContainerStop(ctx, in.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				return fmt.Errorf("停止容器失败: %w", err)
			}
		}
	}
	if err = os.MkdirAll(backupDir(), 0700); err != nil {
		return err
	}
	// 临时文件随机命名且权限 0600，完成并恢复容器之后才原子发布 manifest。
	f, err := os.CreateTemp(backupDir(), ".backup-"+id+"-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(tmp) }()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	hash := sha256.New()
	bounded := &limitedWriter{w: io.MultiWriter(f, hash)}
	gz := gzip.NewWriter(bounded)
	tw := tar.NewWriter(gz)
	defer gz.Close()
	defer tw.Close()
	budget := MaxArchiveBytes
	addJSON := func(name string, value any) error {
		b, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return e
		}
		if int64(len(b)) > budget {
			return fmt.Errorf("元数据超过上限")
		}
		budget -= int64(len(b))
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(b)), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		_, e = tw.Write(b)
		return e
	}
	if err = addJSON("selection.json", p.req); err != nil {
		return err
	}
	for _, in := range p.containers {
		copyIn := in
		// 默认排除运行时环境变量；其他 inspect 配置保留以便人工恢复。
		if in.Config != nil {
			config := *in.Config
			if !p.req.IncludeEnv {
				config.Env = nil
			}
			copyIn.Config = &config
		}
		prefix := "containers/" + in.ID
		if err = addJSON(prefix+"/inspect.json", copyIn); err != nil {
			return err
		}
		m.Items = append(m.Items, types.BackupItem{Kind: "container", ContainerID: in.ID, Source: in.Name, ArchivePath: prefix + "/inspect.json"})
	}
	for i, chosen := range p.mounts {
		s.progress(id, 15+i*50/max(1, len(p.mounts)), fmt.Sprintf("流式备份挂载 %s", mountKey(chosen)), false, false, false)
		source := chosen.Source
		if chosen.Type == "volume" {
			source = chosen.Name
		}
		prefix := fmt.Sprintf("data/%03d", i)
		err = withArchiveHelper(ctx, p.cli, mount.Mount{Type: mount.Type(chosen.Type), Source: source}, "/input", false, func(r io.Reader) error {
			return copyArchive(ctx, tw, r, prefix, &budget)
		})
		if err != nil {
			return fmt.Errorf("挂载 %s 备份失败: %w", mountKey(chosen), err)
		}
		m.Items = append(m.Items, types.BackupItem{Kind: "mount", Source: source, MountKey: mountKey(chosen), ArchivePath: prefix + "/input"})
	}
	if p.req.IncludeCompose {
		if err = s.archiveCompose(ctx, p, tw, &budget, &m); err != nil {
			return err
		}
	}
	// 包内 manifest 不包含包自身的哈希与最终大小，旁路完成标记记录这两项。
	if err = addJSON("manifest.json", m); err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.progress(id, 90, "恢复原运行状态并发布备份", false, false, false)
	err = restore()
	restored = true
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	m.Size = bounded.n
	m.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return publish(id, tmp, m)
}

func (s *Service) archiveCompose(ctx context.Context, p *plan, tw *tar.Writer, budget *int64, m *types.BackupManifest) error {
	seen := map[string]bool{}
	for _, in := range p.containers {
		desc := describeContainer(in)
		if !desc.Compose.Candidate {
			m.Warnings = append(m.Warnings, "容器 "+desc.Name+" 没有 Compose 原文件标签")
			continue
		}
		files := append([]string{}, desc.Compose.ConfigFiles...)
		if p.req.IncludeEnv {
			if desc.Compose.WorkingDir == "" {
				return fmt.Errorf("Compose working_dir 标签缺失，不能定位 .env")
			}
			files = append(files, path.Join(desc.Compose.WorkingDir, ".env"))
		}
		for _, file := range files {
			if !path.IsAbs(file) {
				if desc.Compose.WorkingDir == "" {
					return fmt.Errorf("Compose 标签文件缺少绝对路径和 working_dir")
				}
				file = path.Join(desc.Compose.WorkingDir, file)
			}
			if err := safeSource(file); err != nil {
				return err
			}
			if seen[file] {
				continue
			}
			seen[file] = true
			prefix := fmt.Sprintf("compose/daemon/%03d", len(seen))
			source, input := file, "/input"
			optional := p.req.IncludeEnv && path.Base(file) == ".env"
			if optional {
				source = path.Dir(file)
				if err := safeSource(source); err != nil {
					return err
				}
				input = "/input/.env"
			}
			err := withArchiveHelper(ctx, p.cli, mount.Mount{Type: mount.TypeBind, Source: source}, input, true, func(r io.Reader) error {
				return copyArchive(ctx, tw, r, prefix, budget)
			})
			if optional && errors.Is(err, errOptionalMissing) {
				m.Warnings = append(m.Warnings, "可选环境文件不存在: "+file)
				continue
			}
			if err != nil {
				return fmt.Errorf("目标 daemon 无法读取 Compose 原文件 %s: %w", file, err)
			}
			m.Items = append(m.Items, types.BackupItem{Kind: "compose", ContainerID: in.ID, Source: file, ArchivePath: prefix + "/" + path.Base(input)})
		}
	}
	for i, project := range p.projects {
		files := append([]string{}, project.Files...)
		if p.req.IncludeEnv {
			files = append(files, ".env")
		}
		prefix := fmt.Sprintf("compose/local/%03d", i)
		for _, file := range files {
			if err := s.localFile(ctx, tw, project, file, prefix, budget); err != nil {
				if file == ".env" && os.IsNotExist(err) {
					m.Warnings = append(m.Warnings, "可选环境文件不存在: "+filepath.Join(project.Dir, file))
					continue
				}
				return fmt.Errorf("读取扫描项目 %s 原文件失败: %w", project.Name, err)
			}
			m.Items = append(m.Items, types.BackupItem{Kind: "compose", Source: filepath.Join(project.Dir, file), ArchivePath: prefix + "/" + file})
		}
	}
	return nil
}

// publish 将完成标记最后发布，标记写入失败时移除未完成归档。
func publish(id, tmp string, m types.BackupManifest) error {
	archive, manifest, err := paths(id)
	if err != nil {
		return err
	}
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	mf, err := os.CreateTemp(backupDir(), ".manifest-"+id+"-")
	if err != nil {
		return err
	}
	mtmp := mf.Name()
	defer func() { _ = mf.Close(); _ = os.Remove(mtmp) }()
	if err = mf.Chmod(0600); err != nil {
		return err
	}
	if _, err = mf.Write(manifestBytes); err != nil {
		return err
	}
	if err = mf.Sync(); err != nil {
		return err
	}
	if err = mf.Close(); err != nil {
		return err
	}
	storageMu.Lock()
	defer storageMu.Unlock()
	if err = os.Rename(tmp, archive); err != nil {
		return err
	}
	if err = os.Rename(mtmp, manifest); err != nil {
		_ = os.Remove(archive)
		return err
	}
	return nil
}
