import { useEffect, useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { versionAPI } from '../api/client.js'

/**
 * 检查版本是否需要更新
 * @param {string} currentVersion 当前版本
 * @param {string} latestVersion 最新版本
 * @returns {boolean} 是否需要更新
 */
function shouldUpdate(currentVersion, latestVersion) {
  if (currentVersion === 'unknown' || latestVersion === 'unknown') {
    return false
  }
  
  const current = parseVersion(currentVersion)
  const latest = parseVersion(latestVersion)
  
  if (current === null || latest === null) {
    return false
  }
  
  // 比较 major.minor.patch
  if (latest.major > current.major) return true
  if (latest.major === current.major && latest.minor > current.minor) return true
  if (latest.major === current.major && latest.minor === current.minor && latest.patch > current.patch) return true
  
  return false
}

/**
 * 解析版本号
 * @param {string} version 版本号字符串 (e.g., "1.0.0")
 * @returns {Object|null} 解析后的版本对象或 null
 */
function parseVersion(version) {
  if (!version || typeof version !== 'string') return null
  
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)(?:-.+)?$/)
  if (!match) return null
  
  return {
    major: parseInt(match[1], 10),
    minor: parseInt(match[2], 10),
    patch: parseInt(match[3], 10),
    raw: version,
  }
}

/**
 * 版本检查 Hook
 * 用于检查后端版本，并提示用户是否有更新
 */
export function useVersionCheck() {
  const [showUpdatePrompt, setShowUpdatePrompt] = useState(false)

  // 查询后端版本信息
  const { data: versionData, refetch } = useQuery({
    queryKey: ['version'],
    queryFn: async () => {
      try {
        // 获取本地版本信息
        const localResponse = await versionAPI.getVersion('local')
        
        let backendVersion = 'unknown'
        let buildDate = ''
        
        if (localResponse.data.code === 200 || localResponse.data.code === 0) {
          const localData = localResponse.data.data
          if (localData && typeof localData === 'object') {
            backendVersion = localData.version || 'unknown'
            buildDate = localData.buildDate || ''
          } else if (typeof localData === 'string') {
            backendVersion = localData
          }
        }
        
        // 二进制自更新已下线：不再请求远端版本、不再提示后端更新。
        // 升级统一通过“更新容器镜像”完成。
        return {
          backendVersion,
          remoteVersion: backendVersion,
          buildDate,
          hasBackendUpdate: false
        }
      } catch (error) {
        console.error('获取版本信息失败:', error)
        return {
          backendVersion: 'unknown',
          remoteVersion: 'unknown',
          buildDate: '',
          hasBackendUpdate: false
        }
      }
    },
    refetchInterval: 60000, // 每分钟自动刷新
    refetchOnWindowFocus: false,
    staleTime: 30000 // 30秒内不重新请求
  })

  // 二进制自更新已下线：提示改为通过更新容器镜像升级
  const updateBackend = useCallback(async () => {
    alert('二进制自更新已下线，请通过更新容器镜像进行升级')
  }, [])

  // 手动检查更新
  const checkForUpdates = useCallback(async () => {
    await refetch()
  }, [refetch])

  return {
    // 状态
    showUpdatePrompt,
    
    // 版本数据
    backendVersion: versionData?.backendVersion,
    remoteVersion: versionData?.remoteVersion,
    buildDate: versionData?.buildDate,
    hasBackendUpdate: versionData?.hasBackendUpdate,
    
    // 方法
    setShowUpdatePrompt,
    updateBackend,
    checkForUpdates
  }
}
