package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// 镜像详情按完整实例与镜像 ID 定位，不依赖可漂移的列表索引。
func (b *Bot) renderImageList(chat, message int64, page int) {
	list, err := utiles.GetAllImagesListPerHost(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chat, message, "❌ 获取镜像失败："+escapeHTML(compactName(err.Error())), mainMenuKeyboard())
		return
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Size > list[j].Size })
	page, pages, start, end := pageBounds(len(list), page, 8)
	var rows [][]telegram.InlineKeyboardButton
	for _, img := range list[start:end] {
		icon := "⚪"
		if img.InUsed {
			icon = "🟢"
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: icon + " " + compactName(img.ImageName+":"+img.ImageTag) + " · " + img.SizeFormat, CallbackData: fmt.Sprintf("imgdetail|%s|%s|%d", encodeHostRef(img.HostID), img.ID, page)}})
	}
	rows = append(rows, buildPager("imgpg", page, pages, "menu|home")...)
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "🗑 清理本地镜像", CallbackData: "settings|prune"}})
	b.editOrReplyKeyboard(chat, message, fmt.Sprintf("<b>🐳 镜像</b> · %d 个 · %d/%d\n🟢 使用中　⚪ 未使用", len(list), page+1, pages), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}
func (b *Bot) imageDetail(chat, message int64, host, id string, page int) {
	back := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "⬅ 返回镜像", CallbackData: fmt.Sprintf("imgpg|%d", page)}, {Text: "清理本地镜像", CallbackData: "settings|prune"}}}}
	list, err := utiles.GetAllImagesListPerHost(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chat, message, "❌ 获取镜像失败", back)
		return
	}
	for _, img := range list {
		if img.ID == id && encodeHostRef(img.HostID) == encodeHostRef(host) {
			state := "未使用"
			if img.InUsed {
				state = "使用中"
			}
			text := fmt.Sprintf("<b>🐳 镜像详情</b>\n<code>%s</code>\n实例：%s\n大小：%s · %s\nID：<code>%s</code>", escapeHTML(img.ImageName+":"+img.ImageTag), escapeHTML(compactName(img.HostName)), escapeHTML(img.SizeFormat), state, escapeHTML(img.ID))
			b.editOrReplyKeyboard(chat, message, text, back)
			return
		}
	}
	b.editOrReplyKeyboard(chat, message, "⚠️ 镜像不存在或实例不可用", back)
}

// 备份列表真正分页，回调携带完整文件名，详情不再截断后续备份入口。
func (b *Bot) backupListPage(chat, message int64, page int) {
	list, err := utiles.BackupList(b.svcCtx)
	if err != nil {
		b.editOrReplyKeyboard(chat, message, "❌ 获取备份失败："+escapeHTML(compactName(err.Error())), mainMenuKeyboard())
		return
	}
	page, pages, start, end := pageBounds(len(list), page, 8)
	var rows [][]telegram.InlineKeyboardButton
	for _, name := range list[start:end] {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "📄 " + compactName(name), CallbackData: fmt.Sprintf("bkdetail|%d|%s", page, name)}})
	}
	rows = append(rows, buildPager("bkpg", page, pages, "menu|backup")...)
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "📦 创建备份", CallbackData: "backup|create"}})
	b.editOrReplyKeyboard(chat, message, fmt.Sprintf("<b>📂 配置备份</b> · %d 个 · %d/%d", len(list), page+1, pages), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
	b.rememberMenu(chat, message, fmt.Sprintf("bkpg|%d", page))
}
func (b *Bot) backupDetail(chat, message int64, page int, name string) {
	back := fmt.Sprintf("bkpg|%d", page)
	b.rememberMenu(chat, message, back)
	rows := [][]telegram.InlineKeyboardButton{{{Text: "📥 恢复", CallbackData: "backup|restore|" + name}, {Text: "🗑 删除", CallbackData: "backup|delete|" + name}}, {{Text: "⬅ 返回备份", CallbackData: back}}}
	b.editOrReplyKeyboard(chat, message, "<b>📄 配置备份</b>\n<code>"+escapeHTML(name)+"</code>", &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}
func (b *Bot) backupReturn(chat, message int64) string {
	back := b.returnMenu(chat, message)
	if strings.HasPrefix(back, "bkpg|") {
		return back
	}
	return "bkpg|0"
}
func (b *Bot) backupReturnPage(chat, message int64) {
	parts := strings.Split(b.backupReturn(chat, message), "|")
	page, _ := strconv.Atoi(parts[1])
	b.backupListPage(chat, message, page)
}

// 备份操作仅允许当前目录中的真实普通文件，不允许路径穿越或符号链接出根。
func (b *Bot) validBackupName(name string) bool {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return false
	}
	list, err := utiles.BackupList(b.svcCtx)
	if err != nil {
		return false
	}
	found := false
	for _, entry := range list {
		if entry == name {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	root := os.Getenv("BACKUP_DIR")
	if root == "" {
		root = "/data/backups"
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return false
	}
	target := filepath.Join(root, name)
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel == name
}
