package bot

import (
	"sync"
	"testing"
	"time"

	MyType "github.com/l429609201/dockerCopilot/internal/types"
)

// 纯内存测试：不接 Docker、不向 Telegram 发请求。
func TestUpdateSnapshotOwnershipAndSingleConsumption(t *testing.T) {
	b := &Bot{}
	other := &Bot{}
	item := MyType.Container{HostID: "remote"}
	item.ID = "full-container-id"
	source := []MyType.Container{item}
	token := b.saveUpdateSnapshot(42, source, "menu|updates")
	updateSnapshots.Lock()
	entry := updateSnapshots.entries[token]
	entry.confirm = true
	updateSnapshots.entries[token] = entry
	updateSnapshots.Unlock()
	source[0].ID = "changed"
	if _, ok := other.getUpdateSnapshot(42, token, false); ok {
		t.Fatal("跨 Bot 读取快照")
	}
	if _, ok := b.getUpdateSnapshot(43, token, false); ok {
		t.Fatal("跨聊天读取快照")
	}
	snapshot, ok := b.getUpdateSnapshot(42, token, false)
	if !ok || snapshot.items[0].ID != "full-container-id" {
		t.Fatal("快照随源列表漂移")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	consumed := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := b.getUpdateSnapshot(42, token, true); ok {
				mu.Lock()
				consumed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if consumed != 1 {
		t.Fatalf("确认消费次数=%d", consumed)
	}
}
func TestUpdateSnapshotExpired(t *testing.T) {
	b := &Bot{}
	token := b.saveUpdateSnapshot(1, nil, "menu|updates")
	updateSnapshots.Lock()
	s := updateSnapshots.entries[token]
	s.expires = time.Now().Add(-time.Second)
	updateSnapshots.entries[token] = s
	updateSnapshots.Unlock()
	if _, ok := b.getUpdateSnapshot(1, token, true); ok {
		t.Fatal("过期快照仍能确认")
	}
}
func TestCompactGridBounds(t *testing.T) {
	for _, tc := range []struct{ total, page, wantPage, wantPages, start, end int }{{0, 4, 0, 1, 0, 0}, {17, -1, 0, 3, 0, 8}, {17, 99, 2, 3, 16, 17}} {
		p, n, s, e := pageBounds(tc.total, tc.page, 8)
		if p != tc.wantPage || n != tc.wantPages || s != tc.start || e != tc.end {
			t.Fatalf("错误分页: %d %d %d %d", p, n, s, e)
		}
	}
}
