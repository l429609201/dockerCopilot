package bot

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/l429609201/dockerCopilot/internal/module/containerops"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// sendTagSwitch 展示可切换的镜像标签：列出本地与当前镜像同名的其它 tag 作为按钮，
// 并提供"手动输入标签"入口。切换标签本质是用目标 tag 镜像重建容器。
// messageID > 0 时编辑原消息，否则发送新消息。
func (b *Bot) sendTagSwitch(chatID int64, id, name, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.reply(chatID, "❌ 容器不存在或已被删除")
		return
	}
	hs := "|" + encodeHostRef(hostID)
	// 解析当前镜像名（去掉 tag），优先使用 CreateImage
	curImage := c.CreateImage
	if curImage == "" {
		curImage = c.Image // 降级使用 Image 字段
	}
	repo := tagImageRepository(curImage)

	// 查找该容器所属主机上同名镜像的所有 tag（按主机路由，远程容器查远程镜像）
	images, err := utiles.GetImagesListFromHost(b.svcCtx, hostID)
	var rows [][]telegram.InlineKeyboardButton
	if err == nil {
		seen := map[string]bool{}
		for _, img := range images {
			if img.ImageName != repo || img.ImageTag == "" || img.ImageTag == "None" {
				continue
			}
			full := repo + ":" + img.ImageTag
			if seen[full] || full == curImage {
				continue
			}
			seen[full] = true
			rows = append(rows, []telegram.InlineKeyboardButton{
				{Text: "🏷 " + img.ImageTag, CallbackData: fmt.Sprintf("dotag|%s|%s|%s%s", id, name, img.ImageTag, hs)},
			})
		}
	}

	var t strings.Builder
	t.WriteString(fmt.Sprintf("<b>🏷 切换标签：%s</b>\n", escapeHTML(name)))
	t.WriteString(fmt.Sprintf("当前镜像：<code>%s</code>\n", escapeHTML(curImage)))
	if len(rows) > 0 {
		t.WriteString("\n选择本地已有的标签，或手动输入：")
	} else {
		t.WriteString("\n未找到本地其它标签，可手动输入目标标签：")
	}

	// 手动输入 + 返回
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "✏ 手动输入标签", CallbackData: fmt.Sprintf("tagin|%s|%s%s", id, name, hs)},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "⬅ 返回管理", CallbackData: fmt.Sprintf("panel|%s|%s%s", id, name, hs)},
	})
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	// 如果有 messageID，编辑原消息；否则发送新消息
	if messageID > 0 {
		b.editMessageKeyboard(chatID, messageID, t.String(), kb)
	} else {
		b.replyKeyboard(chatID, t.String(), kb)
	}
}

// doSwitchTag 用指定 tag 重建容器：拼出 repo:tag 后走更新流程。
// messageID > 0 时把进度编辑到原消息（面板点击），否则发新消息（文本输入切标签）。
func (b *Bot) doSwitchTag(chatID int64, id, name, tag, hostID string, messageID int64) {
	c, ok := b.findContainerOnHost(id, hostID)
	if !ok {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 容器不存在或已被删除", b.backToPanelKb(id, name, hostID))
		return
	}
	// 创建引用优先，解析时保留 registry 端口并移除 digest。
	image := c.CreateImage
	if image == "" {
		image = c.Image
	}
	repo := tagImageRepository(image)
	tag = strings.TrimSpace(tag)
	if repo == "" || strings.HasPrefix(image, "sha256:") || !imageTagPattern.MatchString(tag) {
		b.reply(chatID, "❌ 镜像引用或标签无效，请使用有效标签。")
		return
	}
	target := repo + ":" + tag
	taskID, err := containerops.NewForHost(b.svcCtx, hostID).UpdateWithOptions(c.ID, target, containerops.UpdateOptions{})
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID,
			fmt.Sprintf("❌ 提交切换失败：%s", escapeHTML(err.Error())), b.backToPanelKb(id, name, hostID))
		return
	}
	// 复用更新进度监听：把切标签进度持续编辑到原消息上
	go b.watchUpdateProgress(chatID, messageID, taskID, name, target)
}

// promptRename 进入重命名等待：提示用户发送新名称。hostID 记入待处理动作。
func (b *Bot) promptRename(chatID int64, id, name, hostID string) {
	b.setPending(chatID, &pendingAction{kind: "rename", id: id, name: name, hostID: hostID})
	b.reply(chatID, fmt.Sprintf("✏ 请发送容器 <b>%s</b> 的新名称（发送 /cancel 取消）：", escapeHTML(name)))
}

// promptTagInput 进入标签输入等待：提示用户发送目标标签。hostID 记入待处理动作。
func (b *Bot) promptTagInput(chatID int64, id, name, hostID string) {
	b.setPending(chatID, &pendingAction{kind: "tag", id: id, name: name, hostID: hostID})
	b.reply(chatID, fmt.Sprintf("🏷 请发送容器 <b>%s</b> 要切换到的镜像标签（发送 /cancel 取消）：\n例如：<code>latest</code> 或 <code>v1.2.3</code>", escapeHTML(name)))
}

// completePending 根据待输入动作类型完成操作。
// 由文本消息触发，无原始面板 messageID，故结果发新消息并附「返回管理」键盘（用新名称定位面板）。
func (b *Bot) completePending(chatID int64, p *pendingAction, input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		b.reply(chatID, "输入为空，已取消。")
		return
	}
	switch p.kind {
	case "rename":
		// 按容器所属主机执行重命名
		if err := containerops.NewForHost(b.svcCtx, p.hostID).Rename(p.id, input); err != nil {
			b.replyKeyboard(chatID, fmt.Sprintf("❌ 重命名失败：%s", escapeHTML(err.Error())), b.backToPanelKb(p.id, p.name, p.hostID))
			return
		}
		// 重命名成功后容器名已变，用新名称构造返回面板按钮
		b.replyKeyboard(chatID, fmt.Sprintf("✅ 容器已重命名为 <b>%s</b>", escapeHTML(input)), b.backToPanelKb(p.id, input, p.hostID))
	case "tag":
		// 文本输入切标签无面板 messageID，传 0 发新消息
		b.doSwitchTag(chatID, p.id, p.name, input, p.hostID, 0)
	default:
		b.reply(chatID, "未知的待处理动作")
	}
}

// tagImageRepository 去掉 digest 和标签，但不误删 registry 的端口。
func tagImageRepository(image string) string {
	image = strings.TrimSpace(image)
	if i := strings.IndexByte(image, '@'); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndexByte(image, ':'); i > strings.LastIndexByte(image, '/') {
		image = image[:i]
	}
	return image
}

var imageTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
