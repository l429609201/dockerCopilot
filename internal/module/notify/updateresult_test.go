package notify

import (
	"fmt"
	"testing"
	"time"
)

// 隔离内存结果缓存，不连接 Docker、Telegram 或真实配置。
func isolateResults(t *testing.T) {
	t.Helper()
	old := resultStore
	resultStore = &updateResultStore{
		latest:     make(map[string]string),
		executions: make(map[string]resultEntry),
	}
	t.Cleanup(func() { resultStore = old })
}

func TestExecutionSnapshotIdentity(t *testing.T) {
	isolateResults(t)
	first := &RuleUpdateResult{RuleID: "rule", KeepOld: true, Failed: []ResultItem{
		{HostID: "remote", ID: "id1", Name: "real", DisplayName: "real@remote"},
	}}
	SaveRuleUpdateResult(first)
	if first.ExecutionID == "" {
		t.Fatal("执行 ID 未生成")
	}
	first.Failed[0].Name = "mutated"
	second := &RuleUpdateResult{RuleID: "rule", Failed: []ResultItem{{ID: "id2"}}}
	SaveRuleUpdateResult(second)
	got := GetRuleUpdateResultByExecutionID(first.ExecutionID)
	if got == nil || !got.KeepOld || got.Failed[0].Name != "real" || got.Failed[0].Label() != "real@remote" {
		t.Fatalf("历史快照发生变化: %+v", got)
	}
	got.Failed[0].Name = "reader mutation"
	if GetRuleUpdateResultByExecutionID(first.ExecutionID).Failed[0].Name != "real" {
		t.Fatal("读取者可修改存储快照")
	}
	if GetRuleUpdateResult("rule").ExecutionID != second.ExecutionID {
		t.Fatal("旧查询接口未返回最近结果")
	}
	if GetRuleUpdateResultByExecutionID("rule") != nil {
		t.Fatal("执行查询错误回退到规则最新结果")
	}
	SaveRuleUpdateResult(first)
	if GetRuleUpdateResultByExecutionID(first.ExecutionID).Failed[0].Name != "real" {
		t.Fatal("同执行 ID 覆盖了既有快照")
	}
}

func TestExecutionSnapshotBoundsAndExpiry(t *testing.T) {
	isolateResults(t)
	for i := 0; i < maxResults+1; i++ {
		SaveRuleUpdateResult(&RuleUpdateResult{RuleID: fmt.Sprint(i)})
	}
	if len(resultStore.executions) != maxResults || len(resultStore.latest) != maxResults {
		t.Fatal("结果缓存超出容量或遗留规则索引")
	}
	resultStore.prune(time.Now().Add(resultTTL + time.Second))
	if len(resultStore.executions) != 0 || len(resultStore.latest) != 0 {
		t.Fatal("过期结果未清理")
	}
	if (ResultItem{Name: "real"}).Label() != "real" {
		t.Fatal("旧条目的名称回退失败")
	}
}
