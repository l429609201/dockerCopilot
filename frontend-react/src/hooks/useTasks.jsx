import { createContext, useContext, useState, useEffect, useRef, useCallback } from 'react'

const TaskContext = createContext(null)

// 任务中心全局上下文：通过 SSE 实时接收后端「全部后台任务」列表，
// 覆盖容器更新/恢复/镜像拉取/Compose/定时更新/镜像清理等所有任务。
// - addTask：本地乐观占位（SSE 数据到达后自动合并/覆盖）；
// - removeTask：加入本地隐藏集合（后端内存 map 仍在，故前端本地过滤）。
export function TaskProvider({ children }) {
  const [remoteTasks, setRemoteTasks] = useState([]) // 来自 SSE 的后端任务
  const [localTasks, setLocalTasks] = useState([])   // 本地乐观占位（尚未被后端确认）
  const [hidden, setHidden] = useState(() => new Set()) // 被用户清除的任务ID
  const doneCbRef = useRef({}) // 任务完成回调
  const prevDoneRef = useRef({}) // 上一轮各任务 isDone，用于触发 onDone

  const addTask = useCallback(({ id, title, onDone }) => {
    if (!id) return
    if (onDone) doneCbRef.current[id] = onDone
    setLocalTasks(prev => prev.some(t => t.id === id) ? prev
      : [...prev, { id, title: title || '后台任务', percentage: 0, message: '排队中', isDone: false, failed: false }])
    setHidden(prev => { if (!prev.has(id)) return prev; const n = new Set(prev); n.delete(id); return n })
  }, [])

  const removeTask = useCallback((id) => {
    delete doneCbRef.current[id]
    setLocalTasks(prev => prev.filter(t => t.id !== id))
    setHidden(prev => { const n = new Set(prev); n.add(id); return n })
  }, [])

  // 建立 SSE 连接，接收后端全部任务列表
  useEffect(() => {
    const token = localStorage.getItem('docker_copilot_token') || ''
    const base =
      (typeof window !== 'undefined' && window.__API_BASE_URL) ||
      localStorage.getItem('api_base_url') ||
      (typeof window !== 'undefined' ? `${window.location.protocol}//${window.location.host}` : '')
    const url = `${base}/api/progress/stream?token=${encodeURIComponent(token)}`
    const es = new EventSource(url)

    es.onmessage = (ev) => {
      try {
        const arr = JSON.parse(ev.data) // TaskProgress[]
        const mapped = (arr || []).map(d => ({
          id: d.taskID,
          title: d.name || taskTypeLabel(d.taskType) || '后台任务',
          percentage: d.percentage ?? 0,
          message: d.message || '',
          detailMsg: d.detailMsg || '',
          isDone: !!d.isDone,
          failed: !!d.failed,
          canceled: !!d.canceled,
          taskType: d.taskType || '',
          startedAt: d.startedAt || 0,
          endedAt: d.endedAt || 0,
          // 镜像分层进度（仅拉取类任务有值），供任务中心展开显示
          layers: Array.isArray(d.layers) ? d.layers : [],
        }))
        for (const t of mapped) {
          if (t.isDone && prevDoneRef.current[t.id] === false) {
            const cb = doneCbRef.current[t.id]
            if (cb) { try { cb(t) } catch (e) { console.error('任务完成回调失败:', e) } }
          }
          prevDoneRef.current[t.id] = t.isDone
        }
        setRemoteTasks(mapped)
        setLocalTasks(prev => prev.filter(l => !mapped.some(m => m.id === l.id)))
      } catch (e) {
        console.error('解析任务列表失败:', e)
      }
    }

    return () => es.close()
  }, [])

  // 合并：后端任务 + 本地占位，过滤被隐藏的，按开始时间倒序
  const tasks = [...remoteTasks, ...localTasks]
    .filter(t => !hidden.has(t.id))
    .sort((a, b) => (b.startedAt || 0) - (a.startedAt || 0))

  return (
    <TaskContext.Provider value={{ tasks, addTask, removeTask }}>
      {children}
    </TaskContext.Provider>
  )
}

// 任务类型 -> 中文标签（后端未给 name 时兜底）
function taskTypeLabel(type) {
  const map = {
    container_update: '容器更新',
    container_restore: '容器恢复',
    image_pull: '镜像拉取',
    compose_action: 'Compose 操作',
    scheduled_update: '定时更新',
    image_prune: '镜像清理',
    image_check: '检查镜像更新',
  }
  return map[type] || ''
}

export function useTasks() {
  const ctx = useContext(TaskContext)
  if (!ctx) {
    // 未包裹 Provider 时降级为空实现，避免组件崩溃
    return { tasks: [], addTask: () => {}, removeTask: () => {} }
  }
  return ctx
}
