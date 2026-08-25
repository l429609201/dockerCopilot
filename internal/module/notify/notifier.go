package notify

// Notifier 是通知渠道的统一抽象。
// 定时更新、容器更新结果、错误告警等都通过该接口发送，
// 具体实现（Telegram、日志等）可插拔，符合依赖倒置原则。
type Notifier interface {
	// Notify 发送一条通知消息，title 为标题，text 为正文。
	// 实现方应自行处理失败重试与限流，不得阻塞调用方过久。
	Notify(title string, text string)
}

// UpdateItem 是"有可用更新的容器"的中立信息结构。
// 定义在 notify 包（scheduler 与 bot 的公共依赖），避免两侧各自定义
// 同构类型导致接口断言失败、以及 bot↔scheduler 的循环依赖。
type UpdateItem struct {
	ID    string // 容器 ID
	Name  string // 容器名
	Image string // 镜像引用
}

// UpdateNotifier 是"带交互式键盘的更新通知"能力接口。
// 通知渠道（如 Telegram Bot）实现它后，可推送带更新/屏蔽/翻页按钮的通知；
// 未实现时调用方回退为纯文本通知。
type UpdateNotifier interface {
	// NotifyUpdateWithKeyboard 推送带交互式键盘的容器更新通知。
	NotifyUpdateWithKeyboard(items []UpdateItem)
}

// RuleResultNotifier 是"带交互式键盘的定时更新完成通知"能力接口。
// 通知渠道（如 Telegram Bot）实现它后，完成消息正文只展示统计+已更新列表，
// 跳过/失败改由内联按钮按需查看，并支持一键重试全部失败；
// 未实现时调用方回退为纯文本通知（正文铺开三段明细）。
type RuleResultNotifier interface {
	// NotifyRuleResult 推送某规则的执行结果（明细已存入 result store，通过 ruleID 取用）。
	NotifyRuleResult(res *RuleUpdateResult)
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
