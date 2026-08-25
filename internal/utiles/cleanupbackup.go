package utiles

import (
	"context"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/zeromicro/go-zero/core/logx"
)

// backupSuffix 更新/重建时保留旧容器所用的固定中缀。
// 三条更新路径（helper 自更新、UpdateContainerOnHost、containerops.Recreate）
// 统一采用 "{原名}-old-{时间戳}" 命名，便于此处按前缀识别并清理。
const backupSuffix = "-old-"

// CleanupOldBackups 清理指定容器名的历史备份容器，仅保留最近 keep 个。
//
// 背景：delOld=false 时每次更新会把旧容器重命名为 "{name}-old-{时间戳}" 保留，
// 但此前没有任何清理机制，导致备份无限累积（用户反馈"旧容器一直留着"）。
// 本函数在每次更新成功后调用：
//   - keep<=0 时删除该名下所有 -old- 备份（对应 delOld=true，本次不留备份）；
//   - keep>0  时按名称倒序（时间戳字典序即时间序）保留最新的 keep 个，其余删除。
//
// 仅删除已停止的备份容器，避免误删仍在运行的同前缀容器；删除失败只记日志不阻断主流程。
func CleanupOldBackups(cli *client.Client, baseName string, keep int) {
	if cli == nil || baseName == "" {
		return
	}
	ctx := context.Background()
	prefix := baseName + backupSuffix

	list, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		logx.Errorf("清理旧备份容器失败：列出容器出错: %v", err)
		return
	}

	// 收集匹配 "{baseName}-old-*" 的备份容器名（Docker 名带前导 /）
	var backups []string
	for _, c := range list {
		for _, n := range c.Names {
			name := strings.TrimPrefix(n, "/")
			if strings.HasPrefix(name, prefix) {
				backups = append(backups, name)
				break
			}
		}
	}
	if len(backups) == 0 {
		return
	}

	// 名称按时间戳结尾，字典序倒序即"最新在前"，保留前 keep 个
	sort.Sort(sort.Reverse(sort.StringSlice(backups)))
	var toRemove []string
	if keep > 0 && len(backups) > keep {
		toRemove = backups[keep:]
	} else if keep <= 0 {
		toRemove = backups
	}

	for _, name := range toRemove {
		if err := cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil {
			logx.Errorf("清理旧备份容器 %s 失败: %v", name, err)
		} else {
			logx.Infof("已清理旧备份容器: %s", name)
		}
	}
}
