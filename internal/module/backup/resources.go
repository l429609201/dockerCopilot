package backup

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
)

const MaxArchiveBytes int64 = 10 * 1024 * 1024 * 1024
const helperImage = "alpine:3.20"

// Service 不把 daemon 上的任何路径解释为 Copilot 本地路径。
type Service struct {
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func hostID(id string) string {
	if id == "" {
		return appconfig.DockerHostLocalID
	}
	return id
}

func (s *Service) client(id string) (*client.Client, error) {
	client, ok := s.svcCtx.DockerManager.GetClient(id)
	if !ok || client == nil {
		return nil, fmt.Errorf("Docker 实例 %s 无可用连接", hostID(id))
	}
	return client, nil
}

// safeSource 拒绝宿主根目录、内核伪文件系统及 Docker 控制套接字。
func safeSource(source string) error {
	if !path.IsAbs(source) || path.Clean(source) != source || strings.ContainsAny(source, "\x00\r\n") {
		return fmt.Errorf("挂载来源必须是规范绝对路径")
	}
	if source == "/" {
		return fmt.Errorf("禁止备份宿主根目录")
	}
	for _, prefix := range []string{"/proc", "/sys", "/dev"} {
		if source == prefix || strings.HasPrefix(source, prefix+"/") {
			return fmt.Errorf("禁止备份内核或设备路径")
		}
	}
	if strings.HasSuffix(strings.ToLower(source), ".sock") || strings.Contains(source, "/docker.sock") {
		return fmt.Errorf("禁止备份套接字")
	}
	return nil
}

func mountKey(m types.BackupMount) string {
	if m.Type == "volume" {
		return m.Type + ":" + m.Name
	}
	return m.Type + ":" + m.Source
}

func describeMount(m dockertypes.MountPoint) types.BackupMount {
	out := types.BackupMount{Type: string(m.Type), Source: m.Source, Name: m.Name, Destination: m.Destination}
	var err error
	switch m.Type {
	case mount.TypeBind:
		err = safeSource(m.Source)
	case mount.TypeVolume:
		if m.Name == "" || strings.ContainsAny(m.Name, "/\\\x00\r\n") {
			err = fmt.Errorf("卷名称无效")
		}
	default:
		err = fmt.Errorf("仅支持 bind 和 volume 挂载")
	}
	out.Allowed = err == nil
	if err != nil {
		out.Reason = err.Error()
	}
	return out
}

func describeContainer(in dockertypes.ContainerJSON) types.BackupContainer {
	out := types.BackupContainer{
		ID: in.ID, Name: strings.TrimPrefix(in.Name, "/"), Mounts: []types.BackupMount{},
		Compose: types.BackupCompose{ConfigFiles: []string{}},
	}
	if in.State != nil {
		out.Running = in.State.Running
	}
	for _, m := range in.Mounts {
		out.Mounts = append(out.Mounts, describeMount(m))
	}
	if in.Config != nil {
		labels := in.Config.Labels
		out.Compose.Project = labels["com.docker.compose.project"]
		out.Compose.WorkingDir = labels["com.docker.compose.project.working_dir"]
		for _, file := range strings.Split(labels["com.docker.compose.project.config_files"], ",") {
			if file = strings.TrimSpace(file); file != "" {
				out.Compose.ConfigFiles = append(out.Compose.ConfigFiles, file)
			}
		}
		out.Compose.Candidate = len(out.Compose.ConfigFiles) > 0
	}
	return out
}

func (s *Service) Resources(ctx context.Context, id string) (*types.BackupResources, error) {
	client, err := s.client(id)
	if err != nil {
		return nil, err
	}
	list, err := client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := &types.BackupResources{
		HostID: hostID(id), Containers: []types.BackupContainer{}, Projects: []types.BackupProject{},
		Limits: types.BackupLimits{MaxArchiveBytes: MaxArchiveBytes},
		Warnings: []string{"在线备份不保证数据库事务一致性；Compose 候选文件将在执行时由目标实例验证"},
	}
	for _, item := range list {
		if item.Labels["dockercopilot.backup.helper"] == "true" {
			continue
		}
		in, err := client.ContainerInspect(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		out.Containers = append(out.Containers, describeContainer(in))
	}
	if hostID(id) == appconfig.DockerHostLocalID {
		out.Projects = s.projects()
	}
	sort.Slice(out.Containers, func(i, j int) bool { return out.Containers[i].Name < out.Containers[j].Name })
	return out, nil
}
