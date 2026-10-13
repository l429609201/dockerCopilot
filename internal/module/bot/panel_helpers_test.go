package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 纯函数回归不连接 Docker 或 Telegram。
func TestPanelImageRepository(t *testing.T) {
	cases := map[string]string{
		"registry:5000/team/app:latest":        "registry:5000/team/app",
		"registry:5000/team/app@sha256:abcdef": "registry:5000/team/app",
		"registry:5000/team/app:v2@sha:abcdef": "registry:5000/team/app",
		"[::1]:5000/team/app:latest":           "[::1]:5000/team/app",
		"ubuntu":                               "ubuntu",
		"ubuntu:latest":                        "ubuntu",
	}
	for input, want := range cases {
		if got := tagImageRepository(input); got != want {
			t.Errorf("%q: got %q, want %q", input, got, want)
		}
	}
	for _, tag := range []string{"latest", "v1.2-rc_1", strings.Repeat("a", 128)} {
		if !imageTagPattern.MatchString(tag) {
			t.Errorf("valid tag rejected: %q", tag)
		}
	}
	for _, tag := range []string{"", "a/b", "a:b", "@sha256:abc", "-latest", strings.Repeat("a", 129)} {
		if imageTagPattern.MatchString(tag) {
			t.Errorf("invalid tag accepted: %q", tag)
		}
	}
}

func TestPanelTruncNameUTF8(t *testing.T) {
	for _, input := range []string{"中文测试", "😀😀😀😀", "a\xffb", "<&>"} {
		for _, limit := range []int{-1, 0, 1, 2, 5} {
			got := truncName(input, limit)
			if !utf8.ValidString(got) {
				t.Errorf("invalid UTF-8: %q", got)
			}
			if limit <= 0 && got != "" {
				t.Errorf("nonpositive limit: %q", got)
			}
			if limit > 0 && len([]rune(got)) > limit {
				t.Errorf("limit exceeded: %q", got)
			}
		}
	}
	if got := truncName("中文测试", 3); got != "中文…" {
		t.Fatalf("unexpected truncation %q", got)
	}
}

func TestPanelSelfUpdateHandoff(t *testing.T) {
	for _, pair := range [][2]string{{"辅助容器已接管，正在重启本程序", ""}, {"", "辅助容器已接管，正在重启本程序。约10-30秒后请刷新页面"}} {
		if !updateHandedOff(pair[0], pair[1]) {
			t.Fatal("handoff must not be reported as success")
		}
	}
	if updateHandedOff("更新完成", "容器已启动") {
		t.Fatal("normal completion misclassified")
	}
}
