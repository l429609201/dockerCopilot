package notify

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ResultItem 分离真实身份和展示名称，展示字段绝不能用于容器重建。
type ResultItem struct {
	HostID      string
	ID          string
	Name        string
	DisplayName string
	Image       string
	Reason      string
}

// Label 保留远程主机标识，同时兼容没有独立展示名称的调用方。
func (i ResultItem) Label() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	return i.Name
}

// RuleUpdateResult 是一次执行的不可变结果快照，按钮应绑定 ExecutionID。
type RuleUpdateResult struct {
	ExecutionID string
	RuleID      string
	RuleName    string
	KeepOld     bool
	Updated     []ResultItem
	Skipped     []ResultItem
	Failed      []ResultItem
}

const (
	resultTTL  = 24 * time.Hour
	maxResults = 256
)

type resultEntry struct {
	result  *RuleUpdateResult
	expires time.Time
}

// 结果只驻留内存：重启后旧按钮明确过期；容量和有效期防止历史结果无界增长。
type updateResultStore struct {
	mu         sync.Mutex
	latest     map[string]string
	executions map[string]resultEntry
}

var resultStore = &updateResultStore{
	latest:     make(map[string]string),
	executions: make(map[string]resultEntry),
}

// cloneResult 复制切片，避免生产者或读取者修改已发布按钮对应的执行快照。
func cloneResult(res *RuleUpdateResult) *RuleUpdateResult {
	if res == nil {
		return nil
	}
	copy := *res
	copy.Updated = append([]ResultItem(nil), res.Updated...)
	copy.Skipped = append([]ResultItem(nil), res.Skipped...)
	copy.Failed = append([]ResultItem(nil), res.Failed...)
	return &copy
}

func (s *updateResultStore) remove(id string) {
	entry := s.executions[id]
	if entry.result != nil && s.latest[entry.result.RuleID] == id {
		delete(s.latest, entry.result.RuleID)
	}
	delete(s.executions, id)
}

func (s *updateResultStore) prune(now time.Time) {
	for id, entry := range s.executions {
		if !now.Before(entry.expires) {
			s.remove(id)
		}
	}
}

// SaveRuleUpdateResult 为旧生产者补齐执行 ID，保存快照且保留按规则查询最近结果的兼容接口。
// 调用方须在发布通知前保存，之后使用同一个 res.ExecutionID 生成按钮。
func SaveRuleUpdateResult(res *RuleUpdateResult) {
	if res == nil || res.RuleID == "" {
		return
	}
	resultStore.mu.Lock()
	defer resultStore.mu.Unlock()
	if res.ExecutionID == "" {
		res.ExecutionID = uuid.NewString()
	}
	now := time.Now()
	resultStore.prune(now)
	// 相同执行 ID 不允许覆盖快照，防止已发出的按钮改变操作对象。
	if _, exists := resultStore.executions[res.ExecutionID]; exists {
		return
	}
	for len(resultStore.executions) >= maxResults {
		oldestID := ""
		var oldest time.Time
		for id, entry := range resultStore.executions {
			if oldestID == "" || entry.expires.Before(oldest) {
				oldestID, oldest = id, entry.expires
			}
		}
		resultStore.remove(oldestID)
	}
	resultStore.executions[res.ExecutionID] = resultEntry{cloneResult(res), now.Add(resultTTL)}
	resultStore.latest[res.RuleID] = res.ExecutionID
}

// GetRuleUpdateResult 兼容按规则读取最近结果；危险回调必须改用执行 ID 查询。
func GetRuleUpdateResult(ruleID string) *RuleUpdateResult {
	resultStore.mu.Lock()
	defer resultStore.mu.Unlock()
	resultStore.prune(time.Now())
	return cloneResult(resultStore.executions[resultStore.latest[ruleID]].result)
}

// GetRuleUpdateResultByExecutionID 严格读取指定执行，不会回退到同规则的新一轮结果。
func GetRuleUpdateResultByExecutionID(executionID string) *RuleUpdateResult {
	resultStore.mu.Lock()
	defer resultStore.mu.Unlock()
	resultStore.prune(time.Now())
	return cloneResult(resultStore.executions[executionID].result)
}
