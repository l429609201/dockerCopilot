package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// ContainerStat 单个容器的资源采样结果，字段均为前端直接可用的最终值。
type ContainerStat struct {
	ID          string  `json:"id"`          // 容器ID（短ID，12位）
	CPUPercent  float64 `json:"cpuPercent"`  // CPU 使用率百分比
	MemUsed     uint64  `json:"memUsed"`     // 内存使用字节数
	MemLimit    uint64  `json:"memLimit"`    // 内存上限字节数
	MemPercent  float64 `json:"memPercent"`  // 内存使用率百分比
	NetRxBytes  uint64  `json:"netRxBytes"`  // 网络累计下行字节
	NetTxBytes  uint64  `json:"netTxBytes"`  // 网络累计上行字节
	BlkReadByte uint64  `json:"blkReadByte"` // 磁盘累计读字节
	BlkWriteByt uint64  `json:"blkWriteByt"` // 磁盘累计写字节
}

// sampleContainerStats 对单个容器采样一次（stream=false，Docker 返回 pre/cur 两组 CPU 用于计算）。
func sampleContainerStats(ctx context.Context, cli *client.Client, id string) (*ContainerStat, error) {
	resp, err := cli.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var s dockertypes.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}

	stat := &ContainerStat{ID: shortID(id)}
	stat.CPUPercent = calcCPUPercent(&s)

	// 内存：used 需扣除 cache（与 docker stats 命令口径一致）
	memUsed := s.MemoryStats.Usage
	if cache, ok := s.MemoryStats.Stats["cache"]; ok && cache <= memUsed {
		memUsed -= cache
	} else if inactive, ok := s.MemoryStats.Stats["inactive_file"]; ok && inactive <= memUsed {
		memUsed -= inactive
	}
	stat.MemUsed = memUsed
	stat.MemLimit = s.MemoryStats.Limit
	if s.MemoryStats.Limit > 0 {
		stat.MemPercent = float64(memUsed) / float64(s.MemoryStats.Limit) * 100.0
	}

	// 网络：累加所有网卡
	for _, n := range s.Networks {
		stat.NetRxBytes += n.RxBytes
		stat.NetTxBytes += n.TxBytes
	}

	// 磁盘IO：累加 blkio 读写
	for _, b := range s.BlkioStats.IoServiceBytesRecursive {
		switch b.Op {
		case "Read", "read":
			stat.BlkReadByte += b.Value
		case "Write", "write":
			stat.BlkWriteByt += b.Value
		}
	}
	return stat, nil
}

// calcCPUPercent 按 docker stats 口径计算 CPU 使用率。
func calcCPUPercent(s *dockertypes.StatsJSON) float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || sysDelta <= 0 {
		return 0
	}
	onlineCPUs := float64(s.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	return (cpuDelta / sysDelta) * onlineCPUs * 100.0
}

// shortID 取容器短ID（前12位）。
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// StatsStreamHandler 通过 SSE 持续推送所有运行中容器的资源采样。
// EventSource 无法自定义头，故用 query token 校验（复用 validWSToken）。
func StatsStreamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validWSToken(r, svcCtx.Config.Auth.AccessSecret) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "不支持流式响应", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ctx := r.Context()
		cli := svcCtx.DockerClient
		cli.NegotiateAPIVersion(ctx)

		push := func() {
			stats := sampleRunningContainers(ctx, cli)
			data, err := json.Marshal(stats)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		push()

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				push()
			}
		}
	}
}

// sampleRunningContainers 并发采样所有运行中容器，返回结果切片。
func sampleRunningContainers(ctx context.Context, cli *client.Client) []*ContainerStat {
	list, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		logx.Errorf("stats 列出容器失败: %v", err)
		return nil
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make([]*ContainerStat, 0, len(list))
	)
	sampleCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	for _, c := range list {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			st, err := sampleContainerStats(sampleCtx, cli, id)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, st)
			mu.Unlock()
		}(c.ID)
	}
	wg.Wait()
	return results
}