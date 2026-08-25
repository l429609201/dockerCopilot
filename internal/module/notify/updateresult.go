package notify

import "sync"

// ResultItem 是定时更新完成后单个容器的处理结果条目。
// 用于"查看跳过/失败明细"以及"重试全部失败"，需携带足够信息重新提交更新。
type ResultItem struct {
	HostID string // 容器所属 Docker 主机 ID（空/local 表示本地）
	ID     string // 容器 ID
	Name   string // 容器名
	Image  string // 镜像引用
	Reason string // 跳过/失败原因（用于明细展示）
}

// RuleUpdateResult 记录一条定时规则最近一次执行的分类明细。
// Rule 名称用于通知标题回显；三类列表分别对应已更新/已跳过/失败。
type RuleUpdateResult struct {
	RuleID    string
	RuleName  string
	KeepOld   bool // 该规则是否保留旧容器（重试失败时复用同一策略）
	Updated   []ResultItem
	Skipped   []ResultItem
	Failed    []ResultItem
}

// updateResultStore 以 ruleID 为键缓存每条规则"最近一次"执行结果。
// 只保留最近一次：完成消息与其明细/重试按钮均针对最近一次执行，历史结果无需累积。
// scheduler 写入、bot 读取，定义在公共 notify 包避免 bot↔scheduler 循环依赖。
type updateResultStore struct {
	mu   sync.RWMutex
	data map[string]*RuleUpdateResult
}

// 全局单例：进程内共享，重启后清空（结果本就是临时交互态，无需持久化）。
var resultStore = &updateResultStore{data: make(map[string]*RuleUpdateResult)}

// SaveRuleUpdateResult 保存/覆盖某规则最近一次执行结果。
func SaveRuleUpdateResult(res *RuleUpdateResult) {
	if res == nil || res.RuleID == "" {
		return
	}
	resultStore.mu.Lock()
	defer resultStore.mu.Unlock()
	resultStore.data[res.RuleID] = res
}

// GetRuleUpdateResult 读取某规则最近一次执行结果，不存在返回 nil。
func GetRuleUpdateResult(ruleID string) *RuleUpdateResult {
	resultStore.mu.RLock()
	defer resultStore.mu.RUnlock()
	return resultStore.data[ruleID]
}
