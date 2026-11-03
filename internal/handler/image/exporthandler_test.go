package image

import "testing"

// 校验按引用导出时文件名可读且只含安全字符，按 ID 导出时取前 12 位。
func TestExportFilename(t *testing.T) {
	id := "sha256:0123456789abcdef0123"
	cases := map[string]string{
		"nginx:1.25":                 "nginx_1.25.tar",
		"ghcr.io/user/app:v1":        "ghcr.io_user_app_v1.tar",
		"evil\"name:1\r\nX-Hdr: bad": "evil_name_1__X-Hdr__bad.tar",
		id:                           "image-0123456789ab.tar",
	}
	for target, want := range cases {
		if got := exportFilename(target, id); got != want {
			t.Errorf("exportFilename(%q) = %q, want %q", target, got, want)
		}
	}
}

// 校验镜像ID比较兼容 sha256 前缀与短ID，空值不匹配。
func TestSameImageID(t *testing.T) {
	full := "sha256:0123456789abcdef"
	if !sameImageID(full, "0123456789abcdef") || !sameImageID(full, "0123456789ab") {
		t.Fatal("同一镜像的不同ID写法应视为相同")
	}
	if sameImageID(full, "sha256:fedcba") || sameImageID(full, "") {
		t.Fatal("不同镜像或空ID不应视为相同")
	}
}
