// Docker 容器状态的统一口径：分类、标签、颜色、筛选判定。
// 后端 internal/logic/container/containerslistlogic.go 直接把 Docker 原生 State
// 透传为 container.status，因此这里的 value 与 Docker State 严格一一对应。
//
// 抽成独立模块的原因：卡片视图（Containers.jsx）与列表视图（ContainerListRow.jsx）
// 都要用同一套口径，若定义在 Containers.jsx 会让子组件反向导入父组件形成循环依赖。

// CONTAINER_STATES Docker 原生容器状态全集（含中文标签）。
export const CONTAINER_STATES = [
  { value: 'running', label: '运行中' },
  { value: 'paused', label: '已暂停' },
  { value: 'restarting', label: '重启中' },
  { value: 'created', label: '已创建' },
  { value: 'exited', label: '已退出' },
  { value: 'removing', label: '移除中' },
  { value: 'dead', label: '异常' },
]

// STOPPED_STATES 「已停止」的严格定义：真正终止的状态。
// 不含 paused / restarting / created —— 这些容器并未停止，
// 旧代码用 status !== 'running' 判定会把它们全部误归为已停止。
export const STOPPED_STATES = ['exited', 'dead']

// STATE_DOT_COLORS 状态圆点/指示器配色。键名严格对齐 Docker State，
// 注意 Docker 并不存在 'stopped' 状态，已退出实际是 'exited'。
export const STATE_DOT_COLORS = {
  running: 'bg-green-500',
  paused: 'bg-blue-500',
  restarting: 'bg-yellow-500',
  created: 'bg-cyan-500',
  removing: 'bg-orange-500',
  exited: 'bg-red-500',
  dead: 'bg-rose-700',
}

// FILTER_OPTIONS 状态筛选下拉的可选项：全部 + 各原生状态 + 聚合口径。
export const FILTER_OPTIONS = [
  { value: '', label: '全部状态' },
  ...CONTAINER_STATES,
  { value: 'stopped', label: '已停止（退出/异常）' },
  { value: 'update', label: '有更新' },
]

// stateLabel 返回状态的中文展示名。
// 未知状态回退原始值而非笼统写「已停止」，避免语义失真。
export function stateLabel(status) {
  const state = (status || '').toLowerCase()
  return CONTAINER_STATES.find((s) => s.value === state)?.label || status || '未知'
}

// stateDotColor 返回状态对应的圆点背景色类名，未知状态用灰色兜底。
export function stateDotColor(status) {
  return STATE_DOT_COLORS[(status || '').toLowerCase()] || 'bg-gray-500'
}

// matchFilter 统一状态筛选判定，供统计栏、状态下拉、全选结果共用同一套口径。
// filter 为空表示不过滤；'stopped' 与 'update' 是聚合口径，其余为 Docker 原生状态值。
export function matchFilter(container, filter) {
  if (!filter) return true
  if (filter === 'update') return container.haveUpdate
  const state = (container.status || '').toLowerCase()
  if (filter === 'stopped') return STOPPED_STATES.includes(state)
  return state === filter
}
