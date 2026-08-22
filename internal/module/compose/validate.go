package compose

import (
	"fmt"

	"sigs.k8s.io/yaml"
)

// ValidationResult 承载 compose 文件的校验结果。
type ValidationResult struct {
	Valid    bool     `json:"valid"`    // 语法是否合法
	Services []string `json:"services"` // 解析到的服务名
	Warnings []string `json:"warnings"` // 高风险配置警告
	Error    string   `json:"error"`    // 语法错误信息
}

// composeDoc 仅解析我们关心的字段，用于风险检查（不追求完整 schema）。
type composeDoc struct {
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Image       string   `json:"image"`
	Privileged  bool     `json:"privileged"`
	NetworkMode string   `json:"network_mode"`
	Volumes     []any    `json:"volumes"`
	CapAdd      []string `json:"cap_add"`
	Devices     []any    `json:"devices"`
	Pid         string   `json:"pid"`
}

// Validate 校验 compose 内容：先做 YAML 语法解析，再扫描高风险配置。
// 返回结果对象；语法错误时 Valid=false。
func Validate(content []byte) ValidationResult {
	result := ValidationResult{Warnings: []string{}, Services: []string{}}
	var doc composeDoc
	if err := yaml.Unmarshal(content, &doc); err != nil {
		result.Valid = false
		result.Error = "YAML 解析失败：" + err.Error()
		return result
	}
	if len(doc.Services) == 0 {
		result.Valid = false
		result.Error = "未找到任何 service 定义"
		return result
	}
	result.Valid = true
	for name, svc := range doc.Services {
		result.Services = append(result.Services, name)
		result.Warnings = append(result.Warnings, riskWarnings(name, svc)...)
	}
	return result
}

// riskWarnings 检查单个服务的高风险配置并返回警告文案。
func riskWarnings(name string, svc composeService) []string {
	var warnings []string
	if svc.Privileged {
		warnings = append(warnings, fmt.Sprintf("服务 %s 启用了 privileged 特权模式", name))
	}
	if svc.NetworkMode == "host" {
		warnings = append(warnings, fmt.Sprintf("服务 %s 使用 host 网络模式", name))
	}
	if svc.Pid == "host" {
		warnings = append(warnings, fmt.Sprintf("服务 %s 使用 host PID 命名空间", name))
	}
	if len(svc.CapAdd) > 0 {
		warnings = append(warnings, fmt.Sprintf("服务 %s 添加了额外 capabilities: %v", name, svc.CapAdd))
	}
	if len(svc.Devices) > 0 {
		warnings = append(warnings, fmt.Sprintf("服务 %s 映射了宿主机设备", name))
	}
	// 检查危险的宿主机根路径挂载
	for _, v := range svc.Volumes {
		if s, ok := v.(string); ok && isSensitiveMount(s) {
			warnings = append(warnings, fmt.Sprintf("服务 %s 挂载了敏感宿主机路径: %s", name, s))
		}
	}
	return warnings
}

// isSensitiveMount 判断卷映射是否涉及敏感宿主机路径。
func isSensitiveMount(volume string) bool {
	sensitivePrefixes := []string{"/:/", "/etc", "/var/run/docker.sock", "/root", "/proc", "/sys", "/boot"}
	for _, p := range sensitivePrefixes {
		if len(volume) >= len(p) && volume[:len(p)] == p {
			return true
		}
	}
	return false
}

// HasWarnings 判断结果是否包含高风险警告。
func (r ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}
