package icons

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// IconItem 图标配置项（新格式）
type IconItem struct {
	Target     string `json:"target"`     // 目标名称（容器名或镜像名）
	TargetType string `json:"targetType"` // container | image
	IconURL    string `json:"iconUrl"`    // 图标URL
	Priority   int    `json:"priority"`   // 优先级（容器级=1，镜像级=2）
}

// readIconsConfig 读取图标配置文件
func readIconsConfig() ([]IconItem, error) {
	data, err := os.ReadFile(iconsConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []IconItem{}, nil
		}
		return nil, err
	}

	var icons []IconItem
	if err := json.Unmarshal(data, &icons); err != nil {
		return nil, err
	}
	return icons, nil
}

// writeIconsConfig 写入图标配置文件
func writeIconsConfig(icons []IconItem) error {
	_ = os.MkdirAll("/data/config", 0755)
	data, err := json.MarshalIndent(icons, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(iconsConfigPath, data, 0644)
}

// ensureBuiltInIcons 确保内置图标配置存在（DockerCopilot 自身的图标）
func ensureBuiltInIcons() error {
	icons, err := readIconsConfig()
	if err != nil {
		return err
	}

	// 检查是否已存在 DockerCopilot 图标配置
	hasDockerCopilot := false
	for _, item := range icons {
		if item.TargetType == "image" &&
		   (item.Target == "dockercopilot" || item.Target == "ghcr.io/l429609201/dockercopilot") {
			hasDockerCopilot = true
			break
		}
	}

	// 如果不存在，添加内置配置
	if !hasDockerCopilot {
		builtInIcons := []IconItem{
			{
				Target:     "dockercopilot",
				TargetType: "image",
				IconURL:    "/favicon.png",
				Priority:   2,
			},
			{
				Target:     "ghcr.io/l429609201/dockercopilot",
				TargetType: "image",
				IconURL:    "/favicon.png",
				Priority:   2,
			},
		}
		icons = append(icons, builtInIcons...)
		return writeIconsConfig(icons)
	}

	return nil
}

// addOrUpdateIcon 添加或更新图标配置
func addOrUpdateIcon(target, targetType, iconURL string, priority int) error {
	icons, err := readIconsConfig()
	if err != nil {
		return err
	}

	// 查找是否已存在
	found := false
	for i := range icons {
		if icons[i].Target == target && icons[i].TargetType == targetType {
			// 更新现有配置
			icons[i].IconURL = iconURL
			icons[i].Priority = priority
			found = true
			break
		}
	}

	// 如果不存在，则添加
	if !found {
		icons = append(icons, IconItem{
			Target:     target,
			TargetType: targetType,
			IconURL:    iconURL,
			Priority:   priority,
		})
	}

	return writeIconsConfig(icons)
}

func ObtainHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logx.Info("获取图标配置")

		// 确保内置图标配置存在（首次调用时自动添加）
		if err := ensureBuiltInIcons(); err != nil {
			logx.Errorf("初始化内置图标配置失败: %v", err)
		}

		// 读取新格式配置文件（icons.json）
		data, err := os.ReadFile(iconsConfigPath)
		if err != nil {
			if os.IsNotExist(err) {
				logx.Info("配置文件不存在，返回空数组")
				httpx.OkJsonCtx(r.Context(), w, types.Resp{
					Code: 200,
					Msg:  "Success",
					Data: []IconItem{},
				})
				return
			}
			logx.Errorf("读取配置文件失败: %v", err)
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("failed to read config: %v", err))
			return
		}

		var icons []IconItem
		if err := json.Unmarshal(data, &icons); err != nil {
			logx.Errorf("解析配置文件失败: %v", err)
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("failed to parse config: %v", err))
			return
		}

		logx.Infof("成功加载图标配置，共 %d 项", len(icons))

		httpx.OkJsonCtx(r.Context(), w, types.Resp{
			Code: 200,
			Msg:  "Success",
			Data: icons,
		})
	}
}
