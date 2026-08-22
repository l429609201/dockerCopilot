import { createContext, useContext, useState, useEffect, useRef, useCallback } from 'react'
import { progressAPI } from '../api/client.js'

const TaskContext = createContext(null)

/**
 * 全局任务上下文：在应用根层统一轮询所有进行中的任务。
 * 与 useProgress 的区别：轮询不随页面组件卸载而中断，
 * 因此切换页面后任务仍在跟踪，进度浮层始终可见。
 */
export function TaskProvider({ children }) {
  const [tasks, setTasks] = useState([])
  const timerRef = useRef(null)
  // onDone 回调不进 state，避免引用变化触发重渲染
  const doneCbRef = useRef({})

  const addTask = useCallback(({ id, title, onDone }) => {
    if (!id) return
    if (onDone) doneCbRef.current[id] = onDone
    setTasks(prev => {
      if (prev.some(t => t.id === id)) return prev
      return [...prev, { id, title: title || '后台任务', percentage: 0, message: '排队中', isDone: false, failed: false }]
    })
  }, [])

  const removeTask = useCallback((id) => {
    delete doneCbRef.current[id]
    setTasks(prev => prev.filter(t => t.id !== id))
  }, [])

  // 统一轮询：仅在存在未完成任务时开启定时器
  useEffect(() => {
    const pending = tasks.filter(t => !t.isDone)
    if (pending.length === 0) {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
      return
    }
    if (timerRef.current) return

    const poll = async () => {
      // 每轮读取最新的未完成任务列表
      const ids = tasksRef.current.filter(t => !t.isDone).map(t => t.id)
      if (ids.length === 0) return
      const results = await Promise.all(ids.map(async (id) => {
        try {
          const resp = await progressAPI.getProgress(id)
          const body = resp.data
          const d = body?.data || {}
          if (body?.code === 400) {
            return { id, isDone: true, failed: true, message: body?.msg || '任务不存在' }
          }
          return {
            id,
            percentage: d.percentage ?? 0,
            message: d.message || '',
            detailMsg: d.detailMsg || '',
            isDone: !!d.isDone,
            failed: !!d.failed,
            canceled: !!d.canceled,
          }
        } catch (err) {
          return { id, isDone: true, failed: true, message: '查询进度失败' }
        }
      }))

      setTasks(prev => prev.map(t => {
        const r = results.find(x => x.id === t.id)
        if (!r) return t
        // 任务刚完成时触发一次 onDone（如刷新列表）
        if (r.isDone && !t.isDone) {
          const cb = doneCbRef.current[t.id]
          if (cb) { try { cb(r) } catch (e) { console.error('任务完成回调失败:', e) } }
        }
        return { ...t, ...r }
      }))
    }

    poll()
    timerRef.current = setInterval(poll, 1500)
    return () => {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null }
    }
  }, [tasks])

  // 用 ref 持有最新 tasks，避免轮询闭包读到旧值
  const tasksRef = useRef(tasks)
  tasksRef.current = tasks

  return (
    <TaskContext.Provider value={{ tasks, addTask, removeTask }}>
      {children}
    </TaskContext.Provider>
  )
}

export function useTasks() {
  const ctx = useContext(TaskContext)
  if (!ctx) {
    // 未包裹 Provider 时降级为空实现，避免组件崩溃
    return { tasks: [], addTask: () => {}, removeTask: () => {} }
  }
  return ctx
}
