package bot

import (
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"sync"
)

// 页面级互斥避免不同聊天共享网络锁；全局锁仅维护有界索引。
type updatePageVersion struct {
	sync.Mutex
	version uint64
}

var updatePageVersions = struct {
	sync.Mutex
	values map[menuLocationKey]*updatePageVersion
}{values: make(map[menuLocationKey]*updatePageVersion)}

func (b *Bot) updatePageState(chat, message int64) *updatePageVersion {
	updatePageVersions.Lock()
	defer updatePageVersions.Unlock()
	key := menuLocationKey{b, chat, message}
	if state := updatePageVersions.values[key]; state != nil {
		return state
	}
	if len(updatePageVersions.values) >= 2048 {
		for k := range updatePageVersions.values {
			delete(updatePageVersions.values, k)
			break
		}
	}
	state := &updatePageVersion{}
	updatePageVersions.values[key] = state
	return state
}
func (b *Bot) advanceUpdatePage(chat, message int64) uint64 {
	state := b.updatePageState(chat, message)
	state.Lock()
	defer state.Unlock()
	state.version++
	return state.version
}
func (b *Bot) checkUpdatesInBackground(chat, message int64, host string, page int) {
	state := b.updatePageState(chat, message)
	state.Lock()
	state.version++
	version := state.version
	state.Unlock()
	go func() {
		images, err := utiles.GetAllImagesList(b.svcCtx)
		if err != nil {
			return
		}
		b.svcCtx.HubImageInfo.CheckUpdate(images)
		list, err := utiles.GetAllContainers(b.svcCtx)
		if err != nil {
			return
		}
		list = utiles.CheckImageUpdate(b.svcCtx, list)
		items := list[:0:0]
		for _, c := range list {
			if c.Update && (host == "*" || c.HostID == host) {
				items = append(items, c)
			}
		}
		if b.updatePageState(chat, message) != state {
			return
		}
		state.Lock()
		defer state.Unlock()
		if state.version != version {
			return
		}
		token := b.saveUpdateSnapshot(chat, items, "menu|updates")
		b.renderUpdateSnapshot(chat, message, token, page)
	}()
}
