import { useState, useEffect, useCallback } from 'react'
import { hostPathAPI } from '../api/client.js'

// 宿主机路径解析能力 hook。
// 统一封装「引入宿主机路径」功能的可用性判断与路径转换，供创建容器/编辑容器等处复用。
//
// 可用性规则：
//   - 映射功能未启用 → 不可用
//   - auto 模式：需 autoAvailable（能从 DC 自身挂载推导出映射）
//   - custom 模式：需至少一条有效映射
// 目标主机是否本地由调用方结合此可用性一起判断（远程主机 DC 摸不到宿主机文件系统）。
export function useHostPathResolve() {
  const [available, setAvailable] = useState(false) // resolve 能力是否可用
  const [reason, setReason] = useState('')          // 不可用原因（用于置灰 tooltip）
  const [loading, setLoading] = useState(true)

  // 加载映射配置，判断 resolve 是否可用
  const loadConfig = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await hostPathAPI.getConfig()
      const d = resp.data?.data || {}
      if (resp.data?.code !== 200) {
        setAvailable(false)
        setReason(resp.data?.msg || '加载宿主机路径映射配置失败')
        return
      }
      if (!d.enabled) {
        setAvailable(false)
        setReason('未启用宿主机路径映射，请在「项目」页配置')
        return
      }
      const mode = (d.mode || 'auto').toLowerCase()
      if (mode === 'custom') {
        const has = Array.isArray(d.mappings) && d.mappings.some((m) => m.containerPath && m.hostPath)
        setAvailable(has)
        setReason(has ? '' : '自定义映射未配置任何有效规则')
      } else {
        setAvailable(!!d.autoAvailable)
        setReason(d.autoAvailable ? '' : (d.autoReason || '无法从当前容器挂载推导映射'))
      }
    } catch (e) {
      setAvailable(false)
      setReason('加载宿主机路径映射配置失败：' + (e.message || '未知错误'))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadConfig() }, [loadConfig])

  // 将容器内路径解析为宿主机真实路径。成功返回 hostPath 字符串，失败返回 null。
  const resolve = useCallback(async (containerPath) => {
    try {
      const resp = await hostPathAPI.resolve(containerPath)
      const d = resp.data
      if (d?.code === 200 && d.data?.hostPath) {
        return { hostPath: d.data.hostPath, error: '' }
      }
      return { hostPath: null, error: d?.msg || '无法转换为宿主机路径' }
    } catch (e) {
      return { hostPath: null, error: e.response?.data?.msg || e.message || '路径转换失败' }
    }
  }, [])

  return { available, reason, loading, resolve, reload: loadConfig }
}
