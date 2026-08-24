package containerops

import (
	"context"
	"encoding/json"
	"time"

	dockertypes "github.com/docker/docker/api/types"
)

// StatSample 容器资源采样结果（Bot 展示用的精简字段）。
type StatSample struct {
	CPUPercent float64 // CPU 使用率百分比
	MemUsage   uint64  // 内存使用字节数
	MemLimit   uint64  // 内存上限字节数
	MemPercent float64 // 内存使用率百分比
}

// Stats 对单个容器采样一次资源占用（stream=false）。
func (s *Service) Stats(id string) (*StatSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := s.cli().ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var js dockertypes.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&js); err != nil {
		return nil, err
	}

	out := &StatSample{}
	// CPU：按 docker stats 口径计算
	cpuDelta := float64(js.CPUStats.CPUUsage.TotalUsage) - float64(js.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(js.CPUStats.SystemUsage) - float64(js.PreCPUStats.SystemUsage)
	if cpuDelta > 0 && sysDelta > 0 {
		onlineCPUs := float64(js.CPUStats.OnlineCPUs)
		if onlineCPUs == 0 {
			onlineCPUs = float64(len(js.CPUStats.CPUUsage.PercpuUsage))
		}
		if onlineCPUs == 0 {
			onlineCPUs = 1
		}
		out.CPUPercent = (cpuDelta / sysDelta) * onlineCPUs * 100.0
	}

	// 内存：扣除 cache，与 docker stats 口径一致
	memUsed := js.MemoryStats.Usage
	if cache, ok := js.MemoryStats.Stats["cache"]; ok && cache <= memUsed {
		memUsed -= cache
	} else if inactive, ok := js.MemoryStats.Stats["inactive_file"]; ok && inactive <= memUsed {
		memUsed -= inactive
	}
	out.MemUsage = memUsed
	out.MemLimit = js.MemoryStats.Limit
	if js.MemoryStats.Limit > 0 {
		out.MemPercent = float64(memUsed) / float64(js.MemoryStats.Limit) * 100.0
	}
	return out, nil
}
