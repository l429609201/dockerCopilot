package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// HostPathMapperLogic 宿主机路径映射逻辑。
type HostPathMapperLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHostPathMapperLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HostPathMapperLogic {
	return &HostPathMapperLogic{ctx: ctx, svcCtx: svcCtx}
}

// mappingEntry 单条映射（容器内路径 -> 宿主机路径）。
type mappingEntry struct {
	ContainerPath string `json:"containerPath"`
	HostPath      string `json:"hostPath"`
}

// buildAutoMappings 从 DC 自身容器的 Mounts 自动推导映射表。
// Mount.Destination = 容器内路径，Mount.Source = 宿主机真实路径。
// 推导失败（拿不到自身容器/无挂载）时返回错误，调用方据此提示用户改用自定义模式。
func (l *HostPathMapperLogic) buildAutoMappings() ([]mappingEntry, error) {
	// 健壮定位自身容器：cgroup ID → hostname → 容器列表模糊匹配，
	// 规避「cgroup 里的 ID 在当前 daemon inspect 不到（重建/嵌套导致 ID 失效）」的问题。
	inspect, err := utiles.InspectSelfContainer(l.svcCtx)
	if err != nil {
		return nil, fmt.Errorf("读取自身容器挂载信息失败：%v，请改用自定义模式手动配置映射", err)
	}

	mappings := make([]mappingEntry, 0, len(inspect.Mounts))
	for _, m := range inspect.Mounts {
		if m.Destination == "" || m.Source == "" {
			continue
		}
		mappings = append(mappings, mappingEntry{
			ContainerPath: m.Destination,
			HostPath:      m.Source,
		})
	}

	if len(mappings) == 0 {
		return nil, fmt.Errorf("当前容器没有可用的目录挂载，自动推导不可用，请改用自定义模式手动配置映射")
	}
	return mappings, nil
}

// getActiveMappings 根据模式返回当前生效的映射表。
func (l *HostPathMapperLogic) getActiveMappings() ([]mappingEntry, error) {
	cfg := l.svcCtx.AppConfig.Get()
	if strings.EqualFold(cfg.HostPathMapper.Mode, appconfig.HostPathModeCustom) {
		mappings := make([]mappingEntry, 0, len(cfg.HostPathMapper.Mappings))
		for i := range cfg.HostPathMapper.Mappings {
			m := &cfg.HostPathMapper.Mappings[i]
			if m.ContainerPath == "" || m.HostPath == "" {
				continue
			}
			mappings = append(mappings, mappingEntry{ContainerPath: m.ContainerPath, HostPath: m.HostPath})
		}
		if len(mappings) == 0 {
			return nil, fmt.Errorf("自定义映射模式下未配置任何有效映射规则，功能不可用")
		}
		return mappings, nil
	}
	// 默认走自动推导
	return l.buildAutoMappings()
}

// ResolveHostPath 将容器内路径解析为宿主机路径（按最长前缀匹配）。
// 例如：/compose/nginx/conf -> /home/root_nas/docker/nginx/conf
func (l *HostPathMapperLogic) ResolveHostPath(containerPath string) (string, error) {
	cfg := l.svcCtx.AppConfig.Get()
	if !cfg.HostPathMapper.Enabled {
		return "", fmt.Errorf("宿主机路径映射功能未启用")
	}

	mappings, err := l.getActiveMappings()
	if err != nil {
		return "", err
	}

	containerPath = filepath.Clean(containerPath)

	var bestHost, bestContainer string
	maxLen := -1
	for _, m := range mappings {
		cleanContainer := filepath.Clean(m.ContainerPath)
		if containerPath == cleanContainer || strings.HasPrefix(containerPath, cleanContainer+string(filepath.Separator)) {
			if len(cleanContainer) > maxLen {
				bestHost = m.HostPath
				bestContainer = cleanContainer
				maxLen = len(cleanContainer)
			}
		}
	}

	if maxLen < 0 {
		return "", fmt.Errorf("未找到匹配的路径映射规则：%s", containerPath)
	}

	relativePath := strings.TrimPrefix(containerPath, bestContainer)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))
	return filepath.Join(bestHost, relativePath), nil
}

// ValidateAndResolve 解析并校验：返回宿主机路径、是否可访问、原因。
// 校验通过容器内路径对应的实际文件系统 Stat，确认路径真实可用；不可用则功能不给用。
func (l *HostPathMapperLogic) ValidateAndResolve(containerPath string) (*types.Resp, error) {
	if strings.TrimSpace(containerPath) == "" {
		return &types.Resp{Code: 400, Msg: "容器内路径不能为空"}, nil
	}

	hostPath, err := l.ResolveHostPath(containerPath)
	if err != nil {
		return &types.Resp{
			Code: 400,
			Msg:  err.Error(),
			Data: map[string]interface{}{"accessible": false, "reason": err.Error()},
		}, nil
	}

	// 通过容器内路径 Stat 校验实际可访问性（容器内该路径即对应宿主机挂载点）
	cleanContainer := filepath.Clean(containerPath)
	if _, statErr := os.Stat(cleanContainer); statErr != nil {
		reason := fmt.Sprintf("路径不可访问：%s（%v）", cleanContainer, statErr)
		return &types.Resp{
			Code: 400,
			Msg:  reason,
			Data: map[string]interface{}{"hostPath": hostPath, "accessible": false, "reason": reason},
		}, nil
	}

	return &types.Resp{
		Code: 200,
		Msg:  "success",
		Data: map[string]interface{}{
			"containerPath": cleanContainer,
			"hostPath":      hostPath,
			"accessible":    true,
		},
	}, nil
}

// GetConfig 获取映射配置：启用状态、模式、自定义映射，以及自动模式下的推导预览。
func (l *HostPathMapperLogic) GetConfig() *types.Resp {
	cfg := l.svcCtx.AppConfig.Get()
	mode := cfg.HostPathMapper.Mode
	if mode == "" {
		mode = appconfig.HostPathModeAuto
	}

	data := map[string]interface{}{
		"enabled":  cfg.HostPathMapper.Enabled,
		"mode":     mode,
		"mappings": cfg.HostPathMapper.Mappings,
	}

	// 自动模式下附带推导预览，便于前端展示当前生效的映射；推导失败则返回原因供提示。
	if strings.EqualFold(mode, appconfig.HostPathModeAuto) {
		if auto, err := l.buildAutoMappings(); err != nil {
			data["autoAvailable"] = false
			data["autoReason"] = err.Error()
		} else {
			data["autoAvailable"] = true
			data["autoMappings"] = auto
		}
	}

	return &types.Resp{Code: 200, Msg: "success", Data: data}
}

// SaveConfig 保存映射配置（启用状态、模式、自定义映射）。
func (l *HostPathMapperLogic) SaveConfig(req *types.HostPathConfigReq) (*types.Resp, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != appconfig.HostPathModeAuto && mode != appconfig.HostPathModeCustom {
		mode = appconfig.HostPathModeAuto
	}

	// 自定义模式必须至少有一条有效映射，否则功能不可用
	if mode == appconfig.HostPathModeCustom {
		valid := false
		for _, m := range req.Mappings {
			if strings.TrimSpace(m.ContainerPath) != "" && strings.TrimSpace(m.HostPath) != "" {
				valid = true
				break
			}
		}
		if req.Enabled && !valid {
			return &types.Resp{Code: 400, Msg: "自定义模式下请至少配置一条完整的映射（容器内路径 + 宿主机路径）"}, nil
		}
	}

	err := l.svcCtx.AppConfig.Update(func(c *appconfig.AppConfig) error {
		c.HostPathMapper.Enabled = req.Enabled
		c.HostPathMapper.Mode = mode
		mappings := make([]appconfig.PathMapping, 0, len(req.Mappings))
		for _, m := range req.Mappings {
			if strings.TrimSpace(m.ContainerPath) == "" || strings.TrimSpace(m.HostPath) == "" {
				continue
			}
			mappings = append(mappings, appconfig.PathMapping{
				ContainerPath: strings.TrimSpace(m.ContainerPath),
				HostPath:      strings.TrimSpace(m.HostPath),
			})
		}
		c.HostPathMapper.Mappings = mappings
		return nil
	})
	if err != nil {
		return &types.Resp{Code: 500, Msg: "保存配置失败：" + err.Error()}, nil
	}

	return &types.Resp{Code: 200, Msg: "配置已保存并生效"}, nil
}

