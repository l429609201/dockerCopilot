import { createContext, useContext, useState, useCallback, useRef } from 'react'
import { CheckCircle2, XCircle, AlertTriangle, Info, X } from 'lucide-react'

// 全局 Toast（卡片式消息通知）
// 用法：const toast = useToast(); toast.success(resp.msg) / toast.error(msg) / toast.warning(msg) / toast.info(msg)
// 设计对标：顶部居中、圆角卡片、按类型着色、自动消失、可手动关闭、支持堆叠。

const ToastContext = createContext(null)

// 每种类型对应的图标与配色（含暗色模式）
const TOAST_STYLES = {
  success: {
    icon: CheckCircle2,
    cls: 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-700/50',
    iconCls: 'text-emerald-500 dark:text-emerald-400',
  },
  error: {
    icon: XCircle,
    cls: 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-300 border-red-200 dark:border-red-700/50',
    iconCls: 'text-red-500 dark:text-red-400',
  },
  warning: {
    icon: AlertTriangle,
    cls: 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-700/50',
    iconCls: 'text-amber-500 dark:text-amber-400',
  },
  info: {
    icon: Info,
    cls: 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-700/50',
    iconCls: 'text-blue-500 dark:text-blue-400',
  },
}

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]) // { id, type, message }
  const idRef = useRef(0)

  // 移除指定 toast
  const remove = useCallback((id) => {
    setToasts((list) => list.filter((t) => t.id !== id))
  }, [])

  // 新增 toast，duration 毫秒后自动移除（默认 3000ms，0 表示不自动关闭）
  const push = useCallback((type, message, duration = 3000) => {
    if (!message) return
    const id = ++idRef.current
    setToasts((list) => [...list, { id, type, message }])
    if (duration > 0) {
      setTimeout(() => remove(id), duration)
    }
    return id
  }, [remove])

  // 对外暴露的便捷方法
  const value = {
    success: (msg, duration) => push('success', msg, duration),
    error: (msg, duration) => push('error', msg, duration ?? 4000), // 错误默认停留更久
    warning: (msg, duration) => push('warning', msg, duration),
    info: (msg, duration) => push('info', msg, duration),
    remove,
  }

  return (
    <ToastContext.Provider value={value}>
      {children}
      {/* 渲染层：顶部居中固定，堆叠展示 */}
      <div className="fixed top-4 left-1/2 -translate-x-1/2 z-[9999] flex flex-col items-center gap-2 pointer-events-none w-full max-w-[92vw] sm:max-w-md px-2">
        {toasts.map((t) => {
          const style = TOAST_STYLES[t.type] || TOAST_STYLES.info
          const Icon = style.icon
          return (
            <div
              key={t.id}
              role="alert"
              className={`pointer-events-auto w-full flex items-start gap-2.5 px-4 py-3 rounded-xl border shadow-lg backdrop-blur-sm text-sm font-medium animate-toast-in ${style.cls}`}
            >
              <Icon className={`h-5 w-5 flex-shrink-0 mt-0.5 ${style.iconCls}`} />
              <span className="flex-1 break-words leading-snug">{t.message}</span>
              <button
                onClick={() => remove(t.id)}
                className="flex-shrink-0 opacity-60 hover:opacity-100 transition-opacity"
                title="关闭"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider')
  }
  return ctx
}
