package bot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/l429609201/dockerCopilot/internal/module/appconfig"
	"github.com/l429609201/dockerCopilot/internal/module/telegram"
	"github.com/l429609201/dockerCopilot/internal/utiles"
)

// instanceStatus 仅保存可以展示的健康信息，绝不包含连接地址、请求头或原始网络错误。
type instanceStatus struct {
	state                             string
	version                           string
	os                                string
	containers, running, images, cpus int
	memory                            int64
}

// configuredInstances 从配置获取完整清单：EnabledHosts 会遗漏禁用及初始化失败实例。
func (b *Bot) configuredInstances() []appconfig.DockerHost {
	hosts := append([]appconfig.DockerHost(nil), b.svcCtx.AppConfig.Get().DockerHosts...)
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].ID < hosts[j].ID })
	return hosts
}

func (b *Bot) configuredInstance(id string) (appconfig.DockerHost, bool) {
	// 回调必须携带稳定身份，缺失身份不能当成本地。
	if id == "" {
		return appconfig.DockerHost{}, false
	}
	for _, host := range b.configuredInstances() {
		if host.ID == id {
			return host, true
		}
	}
	return appconfig.DockerHost{}, false
}

func instanceLabel(host appconfig.DockerHost) string {
	if host.Name != "" {
		return compactName(host.Name)
	}
	if host.ID == appconfig.DockerHostLocalID {
		return "本地"
	}
	return compactName(host.ID)
}

func (b *Bot) inspectInstance(ctx context.Context, host appconfig.DockerHost) instanceStatus {
	if !host.Enabled {
		return instanceStatus{state: "⚪ 已禁用"}
	}
	cli, ok := b.svcCtx.DockerManager.GetClient(host.ID)
	if !ok || cli == nil {
		return instanceStatus{state: "🔴 连接未建立"}
	}
	info, err := cli.Info(ctx)
	if err != nil {
		return instanceStatus{state: "🔴 不可达或请求超时"}
	}
	return instanceStatus{
		state: "🟢 在线", version: info.ServerVersion, os: info.OperatingSystem,
		containers: info.Containers, running: info.ContainersRunning, images: info.Images,
		cpus: info.NCPU, memory: info.MemTotal,
	}
}

func (b *Bot) replyInstanceOverview(chatID, messageID int64) {
	b.replyInstanceOverviewPage(chatID, messageID, 0)
}

// 每页最多八个健康查询，并共享截止时间，避免不可达主机逐个累积超时。
func (b *Bot) replyInstanceOverviewPage(chatID, messageID int64, page int) {
	hosts := b.configuredInstances()
	page, pages, start, end := pageBounds(len(hosts), page, 8)
	visible := hosts[start:end]
	statuses := make([]instanceStatus, len(visible))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i, host := range visible {
		wg.Add(1)
		go func(i int, host appconfig.DockerHost) {
			defer wg.Done()
			statuses[i] = b.inspectInstance(ctx, host)
		}(i, host)
	}
	wg.Wait()
	var text strings.Builder
	text.WriteString(fmt.Sprintf("<b>🖥 Docker 实例</b> · 共 %d 个 · %d/%d\n", len(hosts), page+1, pages))
	var rows [][]telegram.InlineKeyboardButton
	online, containers, images := 0, 0, 0
	for i, host := range visible {
		status := statuses[i]
		text.WriteString(fmt.Sprintf("• %s · %s\n", escapeHTML(instanceLabel(host)), status.state))
		if status.state == "🟢 在线" {
			online++
			containers += status.containers
			images += status.images
		}
		addGridButton(&rows, telegram.InlineKeyboardButton{Text: instanceLabel(host), CallbackData: "instdetail|" + encodeHostRef(host.ID)}, i)
	}
	text.WriteString(fmt.Sprintf("\n本页可达 %d 个 · 容器 %d · 镜像 %d\n点实例查看详情；配置管理请使用 Web。", online, containers, images))
	rows = append(rows, buildPager("instpage", page, pages, "menu|home")...)
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "🔄 刷新", CallbackData: fmt.Sprintf("instpage|%d", page)}})
	b.editOrReplyKeyboard(chatID, messageID, text.String(), &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (b *Bot) replyInstanceDetail(chatID, messageID int64, hostID string) {
	host, ok := b.configuredInstance(hostID)
	back := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{{Text: "⬅ 实例列表", CallbackData: "menu|instance"}}}}
	if !ok {
		b.editOrReplyKeyboard(chatID, messageID, "⚠️ 实例已移除或身份无效，请重新选择。", back)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status := b.inspectInstance(ctx, host)
	text := fmt.Sprintf("<b>🖥 %s</b>\n%s", escapeHTML(instanceLabel(host)), status.state)
	if status.state == "🟢 在线" {
		text += fmt.Sprintf("\nDocker：%s\n系统：%s\nCPU：%d · 内存：%.1f GiB\n容器：%d（运行 %d）\n镜像：%d", escapeHTML(compactName(status.version)), escapeHTML(compactName(status.os)), status.cpus, float64(status.memory)/(1<<30), status.containers, status.running, status.images)
		back.InlineKeyboard = append([][]telegram.InlineKeyboardButton{{
			{Text: "📦 容器", CallbackData: "instcontainers|" + encodeHostRef(host.ID) + "|0"},
			{Text: "⬆ 更新", CallbackData: "uphost|" + encodeHostRef(host.ID) + "|0"},
		}}, back.InlineKeyboard...)
	}
	back.InlineKeyboard = append(back.InlineKeyboard, []telegram.InlineKeyboardButton{{Text: "🔄 刷新", CallbackData: "instdetail|" + encodeHostRef(host.ID)}})
	b.editOrReplyKeyboard(chatID, messageID, text, back)
}

// 实例容器列表保留稳定主机身份及分页来源，不把远程错误回退到本机。
func (b *Bot) replyInstanceContainers(chatID, messageID int64, hostID string, page int) {
	host, ok := b.configuredInstance(hostID)
	if !ok || !host.Enabled {
		b.editOrReplyKeyboard(chatID, messageID, "⚠️ 实例已移除或禁用，请重新选择。", mainMenuKeyboard())
		return
	}
	list, err := utiles.GetContainerListFromHost(b.svcCtx, hostID)
	if err != nil {
		b.editOrReplyKeyboard(chatID, messageID, "❌ 实例容器列表读取失败，请检查实例连接。", mainMenuKeyboard())
		return
	}
	list = utiles.CheckImageUpdate(b.svcCtx, list)
	sort.Slice(list, func(i, j int) bool { return containerName(list[i]) < containerName(list[j]) })
	page, pages, start, end := pageBounds(len(list), page, pageSize)
	var rows [][]telegram.InlineKeyboardButton
	for i, c := range list[start:end] {
		addGridButton(&rows, telegram.InlineKeyboardButton{Text: containerButton(c), CallbackData: fmt.Sprintf("panel|%s|%s|%s", c.ID, containerName(c), encodeHostRef(hostID))}, i)
	}
	prefix := "instcontainers|" + encodeHostRef(hostID)
	rows = append(rows, buildPager(prefix, page, pages, "instdetail|"+encodeHostRef(hostID))...)
	text := fmt.Sprintf("<b>📦 %s · 容器</b> · %d 个 · %d/%d", escapeHTML(instanceLabel(host)), len(list), page+1, pages)
	b.editOrReplyKeyboard(chatID, messageID, text, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
	b.rememberMenu(chatID, messageID, fmt.Sprintf("%s|%d", prefix, page))
}
