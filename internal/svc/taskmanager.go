package svc

import (
	"context"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// nowMilli 返回当前毫秒时间戳，供任务时间字段统一使用。
func nowMilli() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// runningTask 记录一个正在执行的任务的取消句柄与资源关联信息。
type runningTask struct {
	cancel     context.CancelFunc
	resourceID string
	taskType   string
	startedAt  int64
}

// TaskManager 统一管理异步任务：
//   - 通过带缓冲信号量限制全局并发数，避免同时拉取过多镜像压垮 Docker daemon；
//   - 通过 resourceID 去重，禁止对同一容器重复发起更新；
//   - 保存每个任务的 context 取消函数，支持主动取消。
//
// 该组件职责单一，只负责“调度与生命周期”，具体业务由传入的执行函数完成（SOLID/单一职责）。
type TaskManager struct {
	mu       sync.Mutex
	running  map[string]*runningTask // key: taskID
	byResID  map[string]string       // key: resourceID -> taskID，用于去重
	sem      chan struct{}           // 并发信号量
	maxTasks int
}

// NewTaskManager 创建任务管理器，maxConcurrent 为全局并发上限。
func NewTaskManager(maxConcurrent int) *TaskManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	return &TaskManager{
		running:  make(map[string]*runningTask),
		byResID:  make(map[string]string),
		sem:      make(chan struct{}, maxConcurrent),
		maxTasks: maxConcurrent,
	}
}

// ErrDuplicateResource 表示该资源已有任务在执行。
type taskError string

func (e taskError) Error() string { return string(e) }

const (
	ErrDuplicateResource = taskError("该资源已有任务在执行中")
)

// TryStart 尝试登记并异步执行一个任务。
//   - taskID：任务唯一标识；
//   - resourceID：关联资源，非空时用于去重，为空则不去重；
//   - taskType：任务类型；
//   - fn：实际执行逻辑，接收可取消的 context。
//
// 返回错误表示登记失败（如资源重复），此时不会执行 fn。
func (m *TaskManager) TryStart(taskID, resourceID, taskType string, fn func(ctx context.Context)) error {
	m.mu.Lock()
	if resourceID != "" {
		if existing, ok := m.byResID[resourceID]; ok {
			m.mu.Unlock()
			logx.Errorf("资源 %s 已有任务 %s 在执行，拒绝重复任务 %s", resourceID, existing, taskID)
			return ErrDuplicateResource
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running[taskID] = &runningTask{
		cancel:     cancel,
		resourceID: resourceID,
		taskType:   taskType,
		startedAt:  nowMilli(),
	}
	if resourceID != "" {
		m.byResID[resourceID] = taskID
	}
	m.mu.Unlock()

	go func() {
		// 获取并发令牌；若长时间拿不到令牌，任务会在此排队等待
		m.sem <- struct{}{}
		defer func() {
			<-m.sem
			m.finish(taskID)
			if r := recover(); r != nil {
				logx.Errorf("任务 %s 执行发生 panic 已恢复: %v", taskID, r)
			}
		}()
		fn(ctx)
	}()
	return nil
}

// finish 清理已完成任务的登记信息。
func (m *TaskManager) finish(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.running[taskID]; ok {
		if t.resourceID != "" && m.byResID[t.resourceID] == taskID {
			delete(m.byResID, t.resourceID)
		}
		delete(m.running, taskID)
	}
}

// Cancel 主动取消指定任务，返回是否存在该任务。
func (m *TaskManager) Cancel(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.running[taskID]; ok {
		t.cancel()
		return true
	}
	return false
}

// IsRunning 判断任务是否仍在执行。
func (m *TaskManager) IsRunning(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[taskID]
	return ok
}

// RunningCount 返回当前登记的任务数量（含排队等待令牌的任务）。
func (m *TaskManager) RunningCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}
