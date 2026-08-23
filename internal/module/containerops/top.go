package containerops

import (
	"context"
	"fmt"
)

// TopResult 容器进程列表的统一返回结构。
type TopResult struct {
	Titles    []string   `json:"titles"`    // 列标题（如 UID, PID, PPID, C, STIME, TTY, TIME, CMD）
	Processes [][]string `json:"processes"` // 进程数据行，每行对应 Titles 的列
}

// Top 获取容器内运行的进程列表，调用 Docker ContainerTop API。
// 返回标题行和进程数据矩阵，供前端表格展示（PID/USER/CPU/MEM/COMMAND 等）。
func (s *Service) Top(ctx context.Context, id string) (*TopResult, error) {
	// Docker SDK ContainerTop(ctx, containerID, arguments)
	// arguments 传空数组时使用默认 ps 参数（通常返回 UID/PID/PPID/C/STIME/TTY/TIME/CMD）
	top, err := s.svcCtx.DockerClient.ContainerTop(ctx, id, nil)
	if err != nil {
		return nil, fmt.Errorf("获取容器进程失败: %w", err)
	}

	return &TopResult{
		Titles:    top.Titles,
		Processes: top.Processes,
	}, nil
}
