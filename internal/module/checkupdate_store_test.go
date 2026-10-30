package module

import (
	"path/filepath"
	"testing"

	"github.com/l429609201/dockerCopilot/internal/types"
)

// TestResultKeySeparatesHosts 同 ID 镜像在不同主机上的结果互不覆盖。
func TestResultKeySeparatesHosts(t *testing.T) {
	i := NewImageCheck()
	i.setResult(ResultKey("local", "sha256:a"), ImageCheckList{NeedUpdate: false, Status: StatusLatest})
	i.setResult(ResultKey("remote1", "sha256:a"), ImageCheckList{NeedUpdate: true, Status: StatusNeedUpdate})

	if need, ok := i.NeedUpdate("local", "sha256:a"); !ok || need {
		t.Fatalf("本地结果错误: need=%v ok=%v", need, ok)
	}
	if need, ok := i.NeedUpdate("remote1", "sha256:a"); !ok || !need {
		t.Fatalf("远程结果错误: need=%v ok=%v", need, ok)
	}
	// 空主机ID按本地处理
	if need, ok := i.NeedUpdate("", "sha256:a"); !ok || need {
		t.Fatalf("空主机ID应读取本地结果: need=%v ok=%v", need, ok)
	}
	if _, ok := i.NeedUpdate("remote2", "sha256:a"); ok {
		t.Fatal("未检查的主机不应命中其它主机的结果")
	}
}

// TestResultsPersistAcrossRestart 结果落盘后，新实例能恢复。
func TestResultsPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	first := NewImageCheckWithStore(path)
	first.setResult(ResultKey("remote1", "sha256:a"), ImageCheckList{NeedUpdate: true, Status: StatusNeedUpdate})
	first.saveResults()

	second := NewImageCheckWithStore(path)
	if need, ok := second.NeedUpdate("remote1", "sha256:a"); !ok || !need {
		t.Fatalf("重启后应恢复远程结果: need=%v ok=%v", need, ok)
	}
	if got := second.Snapshot()[ResultKey("remote1", "sha256:a")]; got.CheckedAt.IsZero() {
		t.Fatal("恢复的结果应保留检查时间")
	}
}

// TestPruneKeepsUncheckedHosts 只清理本轮已列出主机上消失的镜像，离线主机结果保留。
func TestPruneKeepsUncheckedHosts(t *testing.T) {
	i := NewImageCheck()
	i.setResult(ResultKey("local", "sha256:old"), ImageCheckList{NeedUpdate: true})
	i.setResult(ResultKey("local", "sha256:cur"), ImageCheckList{NeedUpdate: true})
	i.setResult(ResultKey("offline", "sha256:x"), ImageCheckList{NeedUpdate: true})

	i.pruneHosts(
		map[string]struct{}{"local": {}},
		map[string]struct{}{ResultKey("local", "sha256:cur"): {}},
	)

	snap := i.Snapshot()
	if _, ok := snap[ResultKey("local", "sha256:old")]; ok {
		t.Fatal("已删除镜像的结果应被清理")
	}
	if _, ok := snap[ResultKey("local", "sha256:cur")]; !ok {
		t.Fatal("当前镜像结果应保留")
	}
	if _, ok := snap[ResultKey("offline", "sha256:x")]; !ok {
		t.Fatal("本轮未列出的主机结果应保留")
	}
}

// TestCheckRoundPrunesAndPersists 一轮检查结束后按主机清理并落盘；
// 本地主机本轮只剩自身镜像（会被过滤、不发网络请求），本地旧结果被清理，远程主机结果保留。
func TestCheckRoundPrunesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	i := NewImageCheckWithStore(path)
	i.setResult(ResultKey("local", "sha256:gone"), ImageCheckList{NeedUpdate: true})
	i.setResult(ResultKey("remote1", "sha256:keep"), ImageCheckList{NeedUpdate: true})

	self := types.Image{ImageName: "0nlylty/dockercopilot", ImageTag: "latest", HostID: "local"}
	self.ID = "sha256:self"
	if !i.CheckUpdateWithProgress([]types.Image{self}, nil) {
		t.Fatal("本轮应正常执行")
	}

	reloaded := NewImageCheckWithStore(path)
	if _, ok := reloaded.NeedUpdate("local", "sha256:gone"); ok {
		t.Fatal("本地已不存在的镜像结果应被清理")
	}
	if _, ok := reloaded.NeedUpdate("remote1", "sha256:keep"); !ok {
		t.Fatal("本轮未列出的远程主机结果应保留")
	}
}
