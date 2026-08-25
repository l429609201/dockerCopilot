import React, { useState } from 'react'
import { FolderInput, Loader2 } from 'lucide-react'
import { DirectoryPicker } from './DirectoryPicker.jsx'
import { useHostPathResolve } from '../hooks/useHostPathResolve.jsx'
import { useToast } from '../hooks/useToast.jsx'

// 「引入宿主机路径」按钮 + 目录选择器。
// 浏览 DC 挂载目录（如 /compose）选中后经后端 resolve 转成宿主机真实路径（如 /xxx/data），
// 通过 onPick(hostPath) 回调纯路径给调用方（插入编辑器光标处）。
//
// 置灰规则：resolve 能力不可用 或 目标为远程主机 时禁用。
// props:
//   - isLocal: 目标是否本地 Docker 主机（远程置灰）
//   - onPick: (hostPath) => void 选中并转换成功后的回调
//   - className: 可选，覆盖按钮样式
export function HostPathImportButton({ isLocal = true, onPick, className = '' }) {
  const { available, reason, loading, resolve } = useHostPathResolve()
  const toast = useToast()
  const [showPicker, setShowPicker] = useState(false)
  const [resolving, setResolving] = useState(false)

  // 综合可用性：能力可用 + 本地主机
  const disabled = loading || resolving || !available || !isLocal
  // 置灰时的提示原因
  const disabledReason = !isLocal
    ? '远程主机不支持引入宿主机路径'
    : (!available ? (reason || '宿主机路径映射不可用') : '')

  // 选中容器内目录 → resolve → 回调宿主机路径
  const handleSelect = async (pickedPath) => {
    setResolving(true)
    const { hostPath, error } = await resolve(pickedPath)
    setResolving(false)
    if (hostPath) {
      onPick?.(hostPath)
      toast.success(`已引入宿主机路径：${hostPath}`)
    } else {
      toast.error(error || '无法转换为宿主机路径，请在「项目」页配置映射')
    }
  }

  return (
    <>
      <button
        type="button"
        disabled={disabled}
        onClick={() => setShowPicker(true)}
        title={disabled ? disabledReason : '浏览宿主机映射目录并引入真实路径'}
        className={className || 'inline-flex items-center gap-1 rounded-lg border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-xs text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed'}
      >
        {resolving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <FolderInput className="h-3.5 w-3.5" />}
        引入宿主机路径
      </button>

      {showPicker && (
        <DirectoryPicker
          onClose={() => setShowPicker(false)}
          onSelect={handleSelect}
        />
      )}
    </>
  )
}
