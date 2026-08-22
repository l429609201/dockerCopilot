package notify

// Notifier 是通知渠道的统一抽象。
// 定时更新、容器更新结果、错误告警等都通过该接口发送，
// 具体实现（Telegram、日志等）可插拔，符合依赖倒置原则。
type Notifier interface {
	// Notify 发送一条通知消息，title 为标题，text 为正文。
	// 实现方应自行处理失败重试与限流，不得阻塞调用方过久。
	Notify(title string, text string)
}

// MultiNotifier 组合多个 Notifier，按顺序广播。
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier 创建组合通知器。
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// Add 追加一个通知渠道。
func (m *MultiNotifier) Add(n Notifier) {
	if n != nil {
		m.notifiers = append(m.notifiers, n)
	}
}

// Notify 向所有已注册渠道广播通知。
func (m *MultiNotifier) Notify(title string, text string) {
	for _, n := range m.notifiers {
		if n != nil {
			n.Notify(title, text)
		}
	}
}
