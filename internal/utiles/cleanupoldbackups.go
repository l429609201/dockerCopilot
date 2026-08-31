package utiles

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/zeromicro/go-zero/core/logx"
)

// CleanupBackupsForHost 清理指定主机的容器配置备份，仅保留最近 maxKeep 个。
// 规则：
//   - 只处理该主机对应的 backup-*.json（本地为 backup-<日期>.json；
//     远程为 backup-<hostID>-<日期>.json），不动 .yaml 及其他文件。
//   - 按文件修改时间倒序，超出 maxKeep 的最旧文件被删除。
//   - maxKeep <= 0 表示不限制，直接返回。
//
// 按主机分别计数，避免多 Docker 主机的备份互相挤占。
func CleanupBackupsForHost(hostID string, maxKeep int) (deleted int, err error) {
	if maxKeep <= 0 {
		return 0, nil
	}
	if hostID == "" {
		hostID = appconfig.DockerHostLocalID
	}

	dir := BackupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	// 收集属于该主机的备份文件（含修改时间用于排序）。
	type backupFile struct {
		name    string
		modTime int64
	}
	var files []backupFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isBackupOfHost(name, hostID) {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		files = append(files, backupFile{name: name, modTime: info.ModTime().UnixNano()})
	}

	if len(files) <= maxKeep {
		return 0, nil
	}

	// 按修改时间倒序：最新在前，最旧在后。
	sort.Slice(files, func(i, j int) bool { return files[i].modTime > files[j].modTime })

	// 删除超出 maxKeep 的最旧文件。
	for _, f := range files[maxKeep:] {
		full := filepath.Join(dir, f.name)
		if rmErr := os.Remove(full); rmErr != nil {
			logx.Errorf("清理旧备份失败 %s: %v", f.name, rmErr)
			continue
		}
		deleted++
		logx.Infof("🧹 已清理旧备份: %s", f.name)
	}
	return deleted, nil
}

// isBackupOfHost 判断备份文件名是否属于指定主机的容器配置备份。
//   - 本地主机：backup-<日期>.json，且不含额外的 hostID 段（形如 backup-YYYY-MM-DD.json）。
//   - 远程主机：backup-<hostID>-<日期>.json。
// 仅认 backup- 前缀 + .json 后缀，排除 .yaml 与其他文件。
func isBackupOfHost(name, hostID string) bool {
	if !strings.HasPrefix(name, "backup-") || !strings.HasSuffix(name, ".json") {
		return false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, "backup-"), ".json")
	if hostID == "" || hostID == appconfig.DockerHostLocalID {
		// 本地：backup-<日期>.json，中间应形如 YYYY-MM-DD（不带 hostID 前缀）。
		return isDateLike(mid)
	}
	// 远程：backup-<hostID>-<日期>.json，前缀须精确匹配 hostID- 且余下为日期。
	prefix := hostID + "-"
	if !strings.HasPrefix(mid, prefix) {
		return false
	}
	return isDateLike(strings.TrimPrefix(mid, prefix))
}

// isDateLike 粗判字符串是否形如 YYYY-MM-DD（长度10、两个连字符、其余为数字）。
// 用于区分「本地日期备份」与「远程 hostID-日期备份」，避免 hostID 含连字符时误判。
func isDateLike(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
