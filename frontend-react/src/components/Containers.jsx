import React, { useState } from 'react'
import {
  Play,
  Square,
  RotateCcw,
  RefreshCw,
  Upload,
  Clock,
  Calendar,
  Package,
  X,
  Info,
  LayoutGrid,
  List,
  Edit3,
  Activity,
  FileText,
  TerminalSquare,
  FolderOpen,
  Pencil,
  Network,
  Plus
} from 'lucide-react'
import { containerAPI, progressAPI, imageAPI } from '../api/client.js'
import { cn } from '../utils/cn.js'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { resolveContainerIcon } from '../config/imageLogos.js'
import { ContainerOps } from './ContainerOps.jsx'
import { useFaviconMap } from '../hooks/useFavicon.js'
import { ContainerListRow } from './ContainerListRow.jsx'
import { formatRunningTime } from '../utils/format.js'
import { useTasks } from '../hooks/useTasks.jsx'
import { useContainerStats } from '../hooks/useContainerStats.js'
import { ContainerEditModal } from './ContainerEditModal.jsx'
import { ContainerProcessModal } from './ContainerProcessModal.jsx'
import { ContainerStats } from './ContainerStats.jsx'
import { StatsChart } from './StatsChart.jsx'
import { FileManager } from './FileManager.jsx'
import { CreateContainerModal } from './CreateContainerModal.jsx'
import { IconEditor } from './IconEditor.jsx'
import { ContainerLogs, ContainerConsole } from './ContainerOps.jsx'

export function Containers() {
  const { addTask } = useTasks()
  const queryClient = useQueryClient()
  const [selectedContainer, setSelectedContainer] = useState(null)
  // 【新】独立的日志和控制台弹窗状态
  const [logsTarget, setLogsTarget] = useState(null)
  const [consoleTarget, setConsoleTarget] = useState(null)
  // 文件管理弹窗目标容器
  const [fileTarget, setFileTarget] = useState(null)
  // 创建容器弹窗显示状态
  const [showCreate, setShowCreate] = useState(false)
  // 添加批量操作相关的状态
  const [selectedContainers, setSelectedContainers] = useState([])
  const [isBatchMode, setIsBatchMode] = useState(false)
  // 添加操作状态跟踪
  const [containerActions, setContainerActions] = useState({}) // 跟踪每个容器的操作状态
  const [updateTasks, setUpdateTasks] = useState({}) // 跟踪更新任务
  // 添加筛选状态
  const [filterStatus, setFilterStatus] = useState(null) // null 表示显示全部
  // 视图模式：card 卡片 / list 列表，持久化到 localStorage
  const [viewMode, setViewMode] = useState(() => localStorage.getItem('dc_container_view') || 'card')
  const changeViewMode = (mode) => {
    setViewMode(mode)
    localStorage.setItem('dc_container_view', mode)
  }
  // 通过 SSE 实时订阅容器资源监控（CPU/内存/流量），statsMap 以容器短ID为 key
  const { statsMap } = useContainerStats(true)
  // 容器ID可能是长ID，stats 用短ID，做个安全取值
  const getStat = (id) => statsMap[id] || statsMap[(id || '').slice(0, 12)]

  // 自定义确认弹窗状态
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    onConfirm: null,
    onCancel: null,
    type: 'info' // info, warning, danger
  })

  // 编辑/进程弹窗状态
  const [editTarget, setEditTarget] = useState(null)
  const [processTarget, setProcessTarget] = useState(null)


  // 使用React Query获取容器列表
  const { data: containers = [], isLoading, refetch } = useQuery({
    queryKey: ['containers'],
    queryFn: async () => {
      const response = await containerAPI.getContainers()
      if (response.data.code === 200 || response.data.code === 0) {
        console.log('容器数据:', response.data.data)
        return response.data.data || []
      } else {
        throw new Error(response.data.msg)
      }
    },
    refetchInterval: 10000, // 每10秒自动刷新一次
  })

  // 获取自定义图标配置
  const { data: customIcons = {} } = useQuery({
    queryKey: ['customIcons'],
    queryFn: async () => {
      console.log('[Debug] 开始从服务器获取图标配置...')
      try {
        const response = await imageAPI.getIcons()
        console.log('[Debug] 图标API响应:', response.data)
        if (response.data.code === 200 || response.data.code === 0) {
          const icons = response.data.data || {}
          console.log('[Debug] 获取到的图标数据:', icons)
          // update localStorage
          localStorage.setItem('docker_copilot_image_logos', JSON.stringify(icons))
          return icons
        }
      } catch (err) {
        console.error('[Debug] 获取图标失败:', err)
      }
      return {}
    },
    // 初始数据尝试从localStorage获取，避免闪烁
    initialData: () => {
      const saved = localStorage.getItem('docker_copilot_image_logos')
      if (saved) {
        try {
          const parsed = JSON.parse(saved)
          // 只有当有实际数据时才作为初始数据
          if (Object.keys(parsed).length > 0) {
            return parsed
          }
        } catch (e) {
          console.error('解析本地图标配置失败:', e)
        }
      }
      return undefined
    },
    // 即使有初始数据，也立即在后台刷新
    refetchOnMount: true,
  })

  // 阶段8：批量解析容器站点 favicon（运行中且有暴露端口的容器），按容器id取图标
  const faviconMap = useFaviconMap(containers)

  const handleContainerAction = async (containerId, action) => {
    try {
      // 从列表中定位容器，取得其所属 Docker 主机（多 Docker 管理）
      const hostId = (containers.find(c => c.id === containerId) || {}).hostId
      // 设置操作状态为加载中
      setContainerActions(prev => ({
        ...prev,
        [containerId]: { action, loading: true }
      }))

      switch (action) {
        case 'start':
          await containerAPI.startContainer(containerId, hostId)
          break
        case 'stop':
          await containerAPI.stopContainer(containerId, hostId)
          break
        case 'restart':
          await containerAPI.restartContainer(containerId, hostId)
          break
        default:
          break
      }

      // 立即更新本地状态，提供即时反馈
      queryClient.setQueryData(['containers'], (oldData) => {
        if (!oldData) return oldData

        return oldData.map(container => {
          if (container.id === containerId) {
            let newStatus = container.status
            switch (action) {
              case 'start':
                newStatus = 'running'
                break
              case 'stop':
                newStatus = 'stopped'
                break
              case 'restart':
                newStatus = 'running'
                break
              default:
                break
            }
            return { ...container, status: newStatus }
          }
          return container
        })
      })

      // 清除操作状态
      setContainerActions(prev => {
        const newState = { ...prev }
        delete newState[containerId]
        return newState
      })

      // 延迟刷新以获取最新数据
      setTimeout(() => {
        refetch()
      }, 1500)

    } catch (error) {
      console.error('操作失败:', error)
      // 清除操作状态
      setContainerActions(prev => {
        const newState = { ...prev }
        delete newState[containerId]
        return newState
      })

      // 增加超时错误的处理
      if (error.code === 'ECONNABORTED' || error.message.includes('timeout')) {
        console.error(`操作超时，请稍后手动刷新页面查看操作结果`)
      } else {
        console.error(`操作失败: ${error.response?.data?.msg || error.message}`)
      }
    }
  }

  // 批量操作处理函数
  const handleBatchAction = async (action) => {
    try {
      // 为所有选中的容器设置加载状态
      selectedContainers.forEach(containerId => {
        setContainerActions(prev => ({
          ...prev,
          [containerId]: { action, loading: true }
        }))
      })

      // 立即更新本地状态提供即时反馈
      if (action === 'start' || action === 'stop' || action === 'restart') {
        queryClient.setQueryData(['containers'], (oldData) => {
          if (!oldData) return oldData

          return oldData.map(container => {
            if (selectedContainers.includes(container.id)) {
              let newStatus = container.status
              switch (action) {
                case 'start':
                  newStatus = 'running'
                  break
                case 'stop':
                  newStatus = 'stopped'
                  break
                case 'restart':
                  newStatus = 'running'
                  break
                default:
                  break
              }
              return { ...container, status: newStatus }
            }
            return container
          })
        })
      }

      // 对每个选中的容器执行操作
      for (const containerId of selectedContainers) {
        try {
          const container = containers.find(c => c.id === containerId)
          const hostId = container?.hostId

          switch (action) {
            case 'start':
              await containerAPI.startContainer(containerId, hostId)
              break
            case 'stop':
              await containerAPI.stopContainer(containerId, hostId)
              break
            case 'restart':
              await containerAPI.restartContainer(containerId, hostId)
              break
            case 'update':
              if (container) {
                const response = await containerAPI.updateContainer(
                  containerId,
                  container.name,
                  container.usingImage,
                  true,
                  hostId
                )

                if (response.data.code === 200 || response.data.code === 0) {
                  const taskID = response.data.data?.taskID
                  if (taskID) {
                    // 保存任务ID并开始轮询进度
                    setUpdateTasks(prev => ({
                      ...prev,
                      [containerId]: taskID
                    }))
                    // 调用轮询进度函数
                    pollProgress(containerId, taskID)
                  }
                }
              }
              break
            default:
              break
          }
        } finally {
          // 对于非更新操作，立即清除操作状态
          if (action !== 'update') {
            setContainerActions(prev => {
              const newState = { ...prev }
              delete newState[containerId]
              return newState
            })
          }
        }
      }

      // 如果不是更新操作，延迟刷新以获取最新数据
      if (action !== 'update') {
        setTimeout(() => {
          refetch()
        }, 1500)
      }

      // 清除选中状态
      setSelectedContainers([])
      setIsBatchMode(false)
    } catch (error) {
      console.error('批量操作失败:', error)
      // 清除所有操作状态
      selectedContainers.forEach(containerId => {
        setContainerActions(prev => {
          const newState = { ...prev }
          delete newState[containerId]
          return newState
        })
      })

      // 使用自定义弹窗显示错误信息
      setConfirmModal({
        isOpen: true,
        title: '操作失败',
        message: '批量操作失败: ' + (error.response?.data?.msg || error.message || '未知错误'),
        onConfirm: () => setConfirmModal({ isOpen: false }),
        onCancel: null,
        type: 'danger'
      });
    }
  }

  const handleRenameContainer = async (containerId, newName) => {
    try {
      const hostId = (containers.find(c => c.id === containerId) || {}).hostId
      const response = await containerAPI.renameContainer(containerId, newName, hostId)
      if (response.data.code === 200 || response.data.code === 0) {
        await refetch()
        console.log('重命名成功')
      }
    } catch (error) {
      console.error('重命名容器失败:', error)
      console.error(`重命名失败: ${error.response?.data?.msg || error.message}`)
    }
  }

  const handleUpdateContainer = async (containerId, existingTaskID = null) => {
    try {
      const container = containers.find(c => c.id === containerId)
      if (!container) {
        console.error('容器未找到')
        return
      }

      console.log(`开始更新容器 "${container.name}"，使用镜像: ${container.usingImage}`)

      setContainerActions(prev => ({
        ...prev,
        [containerId]: { action: 'update', loading: true, progress: '正在准备更新...', percentage: 0 }
      }))

      if (existingTaskID) {
        console.log('复用已有更新任务, taskID:', existingTaskID)
        setUpdateTasks(prev => ({
          ...prev,
          [containerId]: existingTaskID
        }))
        pollProgress(containerId, existingTaskID)
        return
      }

      // 注意参数顺序: id, containerName, imageNameAndTag, delOldContainer, hostId
      const response = await containerAPI.updateContainer(
        containerId,
        container.name,
        container.usingImage,
        true,
        container.hostId
      )

      console.log('更新容器响应:', response.data)

      if (response.data.code === 200 || response.data.code === 0) {
        const taskID = response.data.data?.taskID

        if (taskID) {
          console.log('开始轮询进度, taskID:', taskID)
          // 保存任务ID并开始轮询进度
          setUpdateTasks(prev => ({
            ...prev,
            [containerId]: taskID
          }))
          // 同时注册到全局任务浮层，切换页面也能看到该更新进度
          addTask({ id: taskID, title: `更新 ${container.name}`, onDone: () => refetch() })

          pollProgress(containerId, taskID)
        } else {
          // 如果没有返回taskID,说明更新可能立即完成
          setContainerActions(prev => {
            const newState = { ...prev }
            delete newState[containerId]
            return newState
          })
          await refetch()

        }
      } else {
        throw new Error(response.data.msg || '更新失败')
      }
    } catch (error) {
      console.error('更新容器失败:', error)
      setContainerActions(prev => {
        const newState = { ...prev }
        delete newState[containerId]
        return newState
      })

      // 增加超时错误的处理
      if (error.code === 'ECONNABORTED' || error.message.includes('timeout')) {
        console.error(`更新操作已提交，但连接超时。请稍后手动刷新页面查看操作结果`)
        // 即使超时也触发轮询，因为操作可能仍在进行中
        // 这里我们不知道taskID，所以无法启动轮询，只能提示用户稍后查看
      } else {
        // 针对名称冲突提供特定的解决方案
        let errorMessage = error.response?.data?.msg || error.message;
        if (errorMessage.includes('重命名') || errorMessage.includes('name conflict') || errorMessage.includes('名称冲突')) {
          errorMessage += '\n\n检测到容器名称冲突问题，建议解决方案：\n' +
            '1. 手动删除或重命名冲突的容器\n' +
            '2. 使用不同的容器名称进行更新\n' +
            '3. 先停止并重命名当前容器，再进行更新操作';
        }

        // 使用自定义弹窗显示错误信息
        setConfirmModal({
          isOpen: true,
          title: '更新失败',
          message: errorMessage,
          onConfirm: () => setConfirmModal({ isOpen: false }),
          onCancel: null,
          type: 'danger'
        });
      }
    }
  }

  // 轮询进度
  const pollProgress = async (containerId, taskID) => {
    const maxAttempts = 60 // 最多轮询60次 (2分钟)
    let attempts = 0
    let pollTimer = null

    const clearPollState = () => {
      if (pollTimer) {
        clearTimeout(pollTimer)
        pollTimer = null
      }
      setContainerActions(prev => {
        const newState = { ...prev }
        delete newState[containerId]
        return newState
      })
      setUpdateTasks(prev => {
        const newState = { ...prev }
        delete newState[containerId]
        return newState
      })
    }

    const poll = async () => {
      try {
        attempts++
        const response = await progressAPI.getProgress(taskID)
        console.log(`进度查询[${attempts}/${maxAttempts}]:`, response.data)

        const data = response.data

        // 提取进度信息
        let progressMsg = '处理中...'
        let percentage = 0

        if (data.data?.progress) {
          progressMsg = data.data.progress
        } else if (data.data?.message) {
          progressMsg = data.data.message
        } else if (data.msg) {
          progressMsg = data.msg
        }
        // 拉取层实时明细（后端 detailMsg，如"进度Downloading: 12MB/50MB"）
        const detailMsg = data.data?.detailMsg || ''

        // 提取百分比
        if (data.data?.percentage !== undefined) {
          percentage = Math.min(100, Math.max(0, parseFloat(data.data.percentage)))
        } else if (data.data?.percent !== undefined) {
          percentage = Math.min(100, Math.max(0, parseFloat(data.data.percent)))
        } else {
          // 尝试从进度消息中提取百分比
          const percentMatch = progressMsg.match(/(\d+(?:\.\d+)?)\s*%/)
          if (percentMatch) {
            percentage = Math.min(100, Math.max(0, parseFloat(percentMatch[1])))
          } else {
            // 根据轮询次数估算进度
            percentage = Math.min(95, (attempts / maxAttempts) * 100)
          }
        }

        // 检查是否完成 - 兼容多种响应格式
        const status = data.data?.status || data.status
        const isDone = data.data?.isDone === true ||
          status === 'completed' ||
          status === 'success' ||
          status === 'done' ||
          status === 'finish' ||
          status === 'finished'

        // 检查是否失败
        const isFailed = status === 'failed' ||
          status === 'error' ||
          progressMsg.includes('失败') ||
          progressMsg.includes('错误') ||
          data.code === 500 ||
          data.code === 400

        const isCompleted = isDone && !isFailed

        if (isCompleted) {
          // 任务完成 - 立即停止轮询
          console.log('容器更新完成，停止轮询')
          clearPollState()
          await refetch()
          console.log('✅ 容器更新完成!')
          return // 确保不再继续执行
        }

        if (isFailed) {
          // 任务失败 - 立即停止轮询
          console.log('容器更新失败，停止轮询')
          clearPollState()
          // 添加更详细的错误信息
          const errorMsg = data.data?.error || data.data?.message || data.msg || '更新失败'
          console.error(`❌ 更新失败: ${errorMsg}`)

          return // 确保不再继续执行
        }

        // 更新容器操作状态，显示进度
        // 单调保护：轮询/推送可能乱序到达，未完成前进度只增不减，避免视觉回跳
        setContainerActions(prev => {
          const prevPct = prev[containerId]?.percentage || 0
          const shownPct = percentage < prevPct ? prevPct : percentage
          return {
            ...prev,
            [containerId]: {
              action: 'update',
              loading: true,
              progress: progressMsg,
              detailMsg: detailMsg,
              percentage: shownPct
            }
          }
        })

        // 继续轮询
        if (attempts < maxAttempts) {
          pollTimer = setTimeout(poll, 2000) // 2秒后再次查询
        } else {
          clearPollState()
          console.error('⏱️ 更新超时，请检查容器状态')

        }
      } catch (error) {
        console.error('查询进度失败:', error)
        clearPollState()
        console.error(`❌ 更新失败: ${error.response?.data?.msg || error.message}`)
        // 显示网络错误或其他异常情况的友好提示

      }
    }

    // 开始轮询
    poll()
  }

  // 容器选择处理函数
  const toggleContainerSelection = (containerId) => {
    if (selectedContainers.includes(containerId)) {
      setSelectedContainers(selectedContainers.filter(id => id !== containerId))
    } else {
      setSelectedContainers([...selectedContainers, containerId])
    }
  }

  // 全选/取消全选
  const toggleSelectAll = () => {
    if (selectedContainers.length === containers.length) {
      setSelectedContainers([])
    } else {
      setSelectedContainers(containers.map(container => container.id))
    }
  }

  // 获取状态指示器颜色
  const getStatusIndicatorColor = (status) => {
    const statusConfig = {
      running: 'bg-green-500',
      stopped: 'bg-red-500',
      restarting: 'bg-yellow-500',
      paused: 'bg-blue-500'
    }

    return statusConfig[status?.toLowerCase()] || 'bg-gray-500'
  }

  // 获取状态颜色（用于小圆点）
  const getStatusColor = (status) => {
    const statusConfig = {
      running: 'bg-green-500',
      stopped: 'bg-red-500',
      restarting: 'bg-yellow-500',
      paused: 'bg-blue-500'
    }

    return statusConfig[status?.toLowerCase()] || 'bg-gray-500'
  }

  if (isLoading) {
    return (
      <div className="w-full px-4 sm:px-6 py-4">
        <div className="animate-pulse space-y-4">
          {[1, 2, 3].map(i => (
            <div key={i} className="card p-6">
              <div className="flex items-center justify-between">
                <div className="space-y-2">
                  <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-32"></div>
                  <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-24"></div>
                </div>
                <div className="h-6 bg-gray-200 dark:bg-gray-700 rounded w-16"></div>
              </div>
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="w-full px-4 sm:px-6 py-4">
      <style>{`
        @keyframes shimmer {
          0% { background-position: -200% 0; }
          100% { background-position: 200% 0; }
        }
        @keyframes bounceArrow {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-4px); }
        }
      `}</style>

      {/* 自定义确认弹窗 */}
      {confirmModal.isOpen && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-md w-full">
            <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
              <h3 className="text-lg font-medium text-gray-900 dark:text-white">
                {confirmModal.title}
              </h3>
              <button
                onClick={() => {
                  if (confirmModal.onCancel) confirmModal.onCancel();
                  setConfirmModal({ isOpen: false });
                }}
                className="text-gray-400 hover:text-gray-500 dark:text-gray-400 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="px-6 py-4">
              <p className="text-gray-600 dark:text-gray-400">
                {confirmModal.message}
              </p>
            </div>
            <div className="px-6 py-4 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-700/30 flex justify-end space-x-3">
              <button
                onClick={() => {
                  if (confirmModal.onCancel) confirmModal.onCancel();
                  setConfirmModal({ isOpen: false });
                }}
                className="btn-secondary"
              >
                取消
              </button>
              <button
                onClick={() => {
                  if (confirmModal.onConfirm) confirmModal.onConfirm();
                }}
                className={cn(
                  "btn-primary",
                  confirmModal.type === 'danger' && "bg-red-600 hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
                )}
              >
                确认
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 页面标题和操作 */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center mb-4 space-y-4 sm:space-y-0 pt-4 sm:pt-0">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">容器管理</h2>
          <p className="text-gray-600 dark:text-gray-400 mt-1">
            管理您的Docker容器，包括启动、停止、重启等操作
          </p>
        </div>

        {/* 批量操作按钮区域 */}
        {!isBatchMode ? (
          <div className="flex items-center space-x-3">
            <button
              className="btn-primary"
              onClick={() => setShowCreate(true)}
            >
              <Plus className="h-4 w-4 mr-2" />
              创建容器
            </button>

            <button
              className="btn-secondary"
              onClick={() => setIsBatchMode(true)}
            >
              批量操作
            </button>

            <button
              className="btn-secondary"
              onClick={() => refetch()}
            >
              <RefreshCw className="h-4 w-4 mr-2" />
              刷新
            </button>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2 sm:gap-3 w-full sm:w-auto">
            <button
              className="btn-secondary px-3 sm:px-4 py-2"
              onClick={toggleSelectAll}
              title={selectedContainers.length === containers.length ? '取消全选' : '全选'}
            >
              <span className="hidden sm:inline">
                {selectedContainers.length === containers.length ? '取消全选' : '全选'}
              </span>
              <span className="sm:hidden text-sm font-semibold">
                {selectedContainers.length}/{containers.length}
              </span>
            </button>
            <button
              className={`btn-primary flex items-center justify-center px-3 sm:px-4 py-2 gap-1 sm:gap-2 ${selectedContainers.length === 0 ? 'opacity-50 cursor-not-allowed' : ''}`}
              disabled={selectedContainers.length === 0}
              onClick={() => handleBatchAction('start')}
              title="启动"
            >
              <Play className="h-4 w-4 flex-shrink-0" />
              <span className="hidden sm:inline">启动</span>
            </button>
            <button
              className={`btn-secondary flex items-center justify-center px-3 sm:px-4 py-2 gap-1 sm:gap-2 ${selectedContainers.length === 0 ? 'opacity-50 cursor-not-allowed' : ''}`}
              disabled={selectedContainers.length === 0}
              onClick={() => handleBatchAction('stop')}
              title="停止"
            >
              <Square className="h-4 w-4 flex-shrink-0" />
              <span className="hidden sm:inline">停止</span>
            </button>
            <button
              className={`btn-secondary flex items-center justify-center px-3 sm:px-4 py-2 gap-1 sm:gap-2 ${selectedContainers.length === 0 ? 'opacity-50 cursor-not-allowed' : ''}`}
              disabled={selectedContainers.length === 0}
              onClick={() => handleBatchAction('restart')}
              title="重启"
            >
              <RotateCcw className="h-4 w-4 flex-shrink-0" />
              <span className="hidden sm:inline">重启</span>
            </button>
            <button
              className={`btn-secondary flex items-center justify-center px-3 sm:px-4 py-2 gap-1 sm:gap-2 ${selectedContainers.length === 0 ? 'opacity-50 cursor-not-allowed' : ''}`}
              disabled={selectedContainers.length === 0}
              onClick={() => handleBatchAction('update')}
              title="更新"
            >
              <Upload className="h-4 w-4 flex-shrink-0" />
              <span className="hidden sm:inline">更新</span>
            </button>
            <button
              className="btn-danger px-3 sm:px-4 py-2"
              onClick={() => {
                setSelectedContainers([])
                setIsBatchMode(false)
              }}
            >
              <span className="hidden sm:inline">取消</span>
              <span className="sm:hidden">✕</span>
            </button>
          </div>
        )}
      </div>

      {/* 统计信息 */}
      <div className="px-2 sm:px-6 py-4">
        <div className="grid grid-cols-4 gap-0 rounded-3xl overflow-hidden shadow-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          {/* 总容器数 */}
          <button
            onClick={() => setFilterStatus(null)}
            className={cn(
              "p-3 sm:p-5 text-center transition-all duration-300 relative overflow-hidden group border-r border-gray-200 dark:border-gray-700 flex flex-col items-center justify-center",
              filterStatus === null ? "bg-primary-50 dark:bg-primary-900/20" : "hover:bg-gray-50 dark:hover:bg-gray-700/50"
            )}
          >
            <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300"></div>
            <div className="relative">
              <div className="text-2xl sm:text-3xl font-bold text-primary-600 dark:text-primary-400 transition-transform duration-300 group-hover:scale-110">
                {containers.length}
              </div>
              <div className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mt-1">总容器</div>
            </div>
          </button>

          {/* 运行中 */}
          <button
            onClick={() => setFilterStatus('running')}
            className={cn(
              "p-3 sm:p-5 text-center transition-all duration-300 relative overflow-hidden group border-r border-gray-200 dark:border-gray-700 flex flex-col items-center justify-center",
              filterStatus === 'running' ? "bg-green-50 dark:bg-green-900/20" : "hover:bg-gray-50 dark:hover:bg-gray-700/50"
            )}
          >
            <div className="absolute inset-0 bg-gradient-to-br from-green-500/5 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300"></div>
            <div className="relative">
              <div className="text-2xl sm:text-3xl font-bold text-green-600 dark:text-green-400 transition-transform duration-300 group-hover:scale-110">
                {containers.filter(c => c.status === 'running').length}
              </div>
              <div className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mt-1">运行中</div>
            </div>
          </button>

          {/* 已停止 */}
          <button
            onClick={() => setFilterStatus('stopped')}
            className={cn(
              "p-3 sm:p-5 text-center transition-all duration-300 relative overflow-hidden group border-r border-gray-200 dark:border-gray-700 flex flex-col items-center justify-center",
              filterStatus === 'stopped' ? "bg-red-50 dark:bg-red-900/20" : "hover:bg-gray-50 dark:hover:bg-gray-700/50"
            )}
          >
            <div className="absolute inset-0 bg-gradient-to-br from-red-500/5 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300"></div>
            <div className="relative">
              <div className="text-2xl sm:text-3xl font-bold text-red-600 dark:text-red-400 transition-transform duration-300 group-hover:scale-110">
                {containers.filter(c => c.status && c.status.toLowerCase() !== 'running').length}
              </div>
              <div className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mt-1">已停止</div>
            </div>
          </button>

          {/* 有更新 */}
          <button
            onClick={() => setFilterStatus('update')}
            className={cn(
              "p-3 sm:p-5 text-center transition-all duration-300 relative overflow-hidden group flex flex-col items-center justify-center",
              filterStatus === 'update' ? "bg-yellow-50 dark:bg-yellow-900/20" : "hover:bg-gray-50 dark:hover:bg-gray-700/50"
            )}
          >
            <div className="absolute inset-0 bg-gradient-to-br from-yellow-500/5 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300"></div>
            <div className="relative">
              <div className="text-2xl sm:text-3xl font-bold text-yellow-600 dark:text-yellow-400 transition-transform duration-300 group-hover:scale-110">
                {containers.filter(c => c.haveUpdate).length}
              </div>
              <div className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 mt-1">有更新</div>
            </div>
          </button>
        </div>
      </div>

      {/* 容器列表 */}
      <div className="px-2 sm:px-6 py-4">
        {(filterStatus || selectedContainers.length > 0) && (
          <div className="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-sm text-blue-700 dark:text-blue-300">
                  {filterStatus && (
                    <>
                      筛选中：
                      {filterStatus === 'running' && '运行中容器 '}
                      {filterStatus === 'stopped' && '已停止容器 '}
                      {filterStatus === 'update' && '有更新容器 '}
                    </>
                  )}
                  {!filterStatus && selectedContainers.length > 0 && (
                    <>
                      已选中 {selectedContainers.length} 个容器（使用 Ctrl/Cmd+点击 来切换选择，或直接点击卡片打开详情）
                    </>
                  )}
                  {filterStatus && selectedContainers.length > 0 && (
                    <>
                      &nbsp;·&nbsp;已选中 {selectedContainers.length} 个容器
                    </>
                  )}
                </span>
                {filterStatus && (
                  <>
                    <button
                      onClick={() => {
                        setFilterStatus(null)
                        setSelectedContainers([])
                        setIsBatchMode(false)
                      }}
                      className="px-2 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-300 hover:text-blue-800 dark:hover:text-blue-100 bg-blue-100 dark:bg-blue-800/50 rounded transition-colors"
                    >
                      清除筛选
                    </button>
                    <button
                      onClick={() => {
                        const filteredContainers = containers.filter((container) => {
                          if (!filterStatus) return true
                          if (filterStatus === 'running') return container.status && container.status.toLowerCase() === 'running'
                          if (filterStatus === 'stopped') return container.status && container.status.toLowerCase() !== 'running'
                          if (filterStatus === 'update') return container.haveUpdate
                          return true
                        })
                        setSelectedContainers(filteredContainers.map(c => c.id))
                        setIsBatchMode(true)
                      }}
                      className="px-2 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-300 hover:text-blue-800 dark:hover:text-blue-100 bg-blue-100 dark:bg-blue-800/50 rounded transition-colors"
                    >
                      全选结果
                    </button>
                  </>
                )}
              </div>
              {selectedContainers.length > 0 && !filterStatus && (
                <button
                  onClick={() => setSelectedContainers([])}
                  className="px-2 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-300 hover:text-blue-800 dark:hover:text-blue-100 bg-blue-100 dark:bg-blue-800/50 rounded transition-colors"
                >
                  取消选择
                </button>
              )}
            </div>
          </div>
        )}

        {/* 视图切换工具栏：卡片 / 列表 */}
        {containers.length > 0 && (
          <div className="flex justify-end mb-3">
            <div className="inline-flex rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
              <button
                onClick={() => changeViewMode('card')}
                className={cn("flex items-center gap-1 px-3 py-1.5 text-sm transition-colors",
                  viewMode === 'card' ? "bg-primary-600 text-white" : "bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700")}
                title="卡片视图"
              >
                <LayoutGrid className="h-4 w-4" /> 卡片
              </button>
              <button
                onClick={() => changeViewMode('list')}
                className={cn("flex items-center gap-1 px-3 py-1.5 text-sm transition-colors",
                  viewMode === 'list' ? "bg-primary-600 text-white" : "bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700")}
                title="列表视图"
              >
                <List className="h-4 w-4" /> 列表
              </button>
            </div>
          </div>
        )}

        {containers.length > 0 ? (
          <div className={cn(
            viewMode === 'list'
              ? "flex flex-col gap-2"
              : "grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
          )}>
            {containers
              .filter((container) => {
                if (!filterStatus) return true
                if (filterStatus === 'running') return container.status && container.status.toLowerCase() === 'running'
                if (filterStatus === 'stopped') return container.status && container.status.toLowerCase() !== 'running'
                if (filterStatus === 'update') return container.haveUpdate
                return true
              })
              .map((container) => {
                const isSelected = selectedContainers.includes(container.id)
                // 列表模式：渲染横向一条的列表行
                if (viewMode === 'list') {
                  // 列表模式与卡片模式共用同一解析逻辑，保证图标一致
                  const iconUrl = resolveContainerIcon(container, faviconMap, customIcons)

                  return (
                    <ContainerListRow
                      key={container.id}
                      container={container}
                      iconUrl={iconUrl}
                      selected={isSelected}
                      batchMode={isBatchMode}
                      actionState={containerActions[container.id]}
                      stat={getStat(container.id)}
                      onOpen={(c) => setSelectedContainer(c)}
                      onToggleSelect={toggleContainerSelection}
                      onAction={handleContainerAction}
                      onUpdate={handleUpdateContainer}
                      onOps={(tab) => setOpsTarget({ container, tab })}
                      onFiles={() => setFileTarget(container)}
                      onEdit={(c) => setEditTarget(c)}
                      onProcess={(c) => setProcessTarget(c)}
                    />
                  )
                }
                return (
                  <div key={container.id} className="group">
                    {/* 容器卡片 - 简化设计，点击调起详情 */}
                    <div
                      onClick={(e) => {
                        // 如果启用批量模式，点击选择；否则打开详情
                        if (e.metaKey || e.ctrlKey || isBatchMode) {
                          e.stopPropagation()
                          toggleContainerSelection(container.id)
                        } else {
                          setSelectedContainer(container)
                        }
                      }}
                      className={cn(
                        "card relative overflow-hidden transition-all duration-200 hover:shadow-lg border rounded-2xl p-5 cursor-pointer active:scale-98",
                        isSelected
                          ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20 shadow-md"
                          : "border-gray-200 dark:border-gray-700 hover:border-primary-300 dark:hover:border-primary-600"
                      )}
                    >
                      {/* 背景进度条 */}
                      {containerActions[container.id]?.loading && containerActions[container.id]?.action === 'update' && (
                        <div className="absolute inset-0 pointer-events-none rounded-2xl overflow-hidden">
                          <div
                            className="absolute top-0 left-0 bottom-0 bg-gradient-to-r from-primary-500/30 via-primary-400/30 to-primary-500/30 transition-all duration-500 ease-out"
                            style={{
                              width: `${containerActions[container.id].percentage || 0}%`
                            }}
                          >
                            <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent animate-shimmer"
                              style={{
                                backgroundSize: '200% 100%',
                                animation: 'shimmer 2s infinite linear'
                              }} />
                          </div>
                        </div>
                      )}

                      {/* NEW (有更新时显示) */}
                      {container.haveUpdate && (
                        <div className="absolute -top-[2px] -right-[2px] w-[80px] h-[80px] pointer-events-none overflow-hidden z-20 rounded-tr-2xl">
                          <div className="absolute top-0 right-0 w-full h-full flex items-center justify-center">
                            <div className="absolute transform rotate-45 translate-x-[26px] -translate-y-[26px] w-[120px] h-[24px] bg-gradient-to-r from-yellow-400 to-yellow-500 dark:from-yellow-500 dark:to-yellow-600 shadow-sm flex items-center justify-center">
                              <span className="relative text-[10px] font-bold text-white tracking-widest uppercase w-full text-center">
                                NEW
                                {/* 流光效果 */}
                                <div className="absolute top-0 left-0 animate-flow-light"></div>
                              </span>
                            </div>
                          </div>
                        </div>
                      )}

                      <div className="relative z-10 flex items-center gap-3">
                        {/* 图标 */}
                        <div className="flex-shrink-0">
                          {(() => {
                            // 与列表模式共用同一解析逻辑，保证图标一致
                            const iconUrl = resolveContainerIcon(container, faviconMap, customIcons);

                            if (iconUrl) {
                              return (
                                <img
                                  src={iconUrl}
                                  alt={container.name}
                                  className="h-12 w-12 rounded-xl object-cover shadow-sm flex-shrink-0"
                                  onError={(e) => {
                                    e.target.style.display = 'none';
                                    e.target.parentElement.innerHTML = `
                                    <div class="h-12 w-12 bg-gradient-to-r from-blue-500 to-purple-600 rounded-xl flex items-center justify-center shadow-sm">
                                      <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-6 w-6 text-white">
                                        <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"></path>
                                      </svg>
                                    </div>
                                  `;
                                  }}
                                />
                              );
                            } else {
                              return (
                                <div className="h-12 w-12 bg-gradient-to-r from-blue-500 to-purple-600 rounded-xl flex items-center justify-center shadow-sm flex-shrink-0">
                                  <Package className="h-6 w-6 text-white" />
                                </div>
                              );
                            }
                          })()}
                        </div>

                        {/* 状态指示器（放在图标和信息之间） */}
                        <div className="flex-shrink-0 flex items-center">
                          <div className={cn(
                            "w-1 h-8 rounded-full",
                            getStatusIndicatorColor(container.status)
                          )} />
                        </div>

                        {/* 容器信息 */}
                        <div className="flex-1 min-w-0">
                          <div className="flex items-start justify-between gap-2">
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-1.5">
                                <h3 className="font-semibold text-gray-900 dark:text-white truncate text-base group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors">
                                  {container.name}
                                </h3>
                                {/* 来源主机标识：非本地容器才展示，避免本地冗余 */}
                                {container.hostName && container.hostId && container.hostId !== 'local' && (
                                  <span className="flex-shrink-0 inline-flex items-center gap-0.5 rounded bg-purple-100 dark:bg-purple-900/40 text-purple-600 dark:text-purple-300 px-1.5 py-0.5 text-[10px] font-medium">
                                    <Network className="h-2.5 w-2.5" />
                                    {container.hostName}
                                  </span>
                                )}
                              </div>
                              <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">
                                {container.usingImage}
                              </p>
                            </div>
                          </div>

                          {/* 统一高度的信息行 - 显示运行时间或更新进度 */}
                          <div className="h-5 mt-1">
                            {containerActions[container.id]?.loading && containerActions[container.id]?.percentage !== undefined ? (
                              <div className="space-y-1">
                                {/* 简洁进度条 - 只显示百分比和进度条 */}
                                <div className="flex items-center gap-2">
                                  <div className="flex-1 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                                    <div
                                      className="h-full bg-gradient-to-r from-blue-500 to-blue-600 transition-all duration-300 rounded-full"
                                      style={{ width: `${containerActions[container.id].percentage}%` }}
                                    />
                                  </div>
                                  <span className="text-xs font-medium text-blue-600 dark:text-blue-400 min-w-[3ch] text-right">
                                    {containerActions[container.id].percentage}%
                                  </span>
                                </div>
                              </div>
                            ) : container.status === 'running' ? (
                              <div className="text-xs text-gray-500 dark:text-gray-400">
                                运行: {formatRunningTime(container.runningTime)}
                              </div>
                            ) : (
                              <div className="text-xs text-gray-500 dark:text-gray-400">
                                状态: 已停止
                              </div>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* 资源监控（运行中容器）：CPU/内存/上下行流量 */}
                      {container.status === 'running' && getStat(container.id) && (
                        <div className="relative z-10 mt-3 pt-3 border-t border-gray-100 dark:border-gray-700/50">
                          <ContainerStats stat={getStat(container.id)} variant="card" />
                        </div>
                      )}

                      {/* 操作按钮栏 - 底部 4 列网格排列（运行中共 8 个按钮，2 行） */}
                      {!isBatchMode && (
                        <div className="grid grid-cols-4 gap-1.5 mt-3 pt-3 border-t border-gray-100 dark:border-gray-700/50">
                          {containerActions[container.id]?.loading ? (
                            <div className="col-span-4 flex flex-col gap-0.5 px-2 py-1.5 bg-primary-50 dark:bg-primary-900/20 rounded-lg border border-primary-200 dark:border-primary-800">
                              <div className="flex items-center justify-center gap-2 whitespace-nowrap">
                                <RefreshCw className="h-4 w-4 animate-spin text-primary-600 dark:text-primary-400" />
                                <span className="text-xs font-medium text-primary-600 dark:text-primary-400">
                                  {containerActions[container.id].action === 'start' && '启动中'}
                                  {containerActions[container.id].action === 'stop' && '停止中'}
                                  {containerActions[container.id].action === 'restart' && '重启中'}
                                  {containerActions[container.id].action === 'update' && `更新中${containerActions[container.id].percentage ? ` ${Math.round(containerActions[container.id].percentage)}%` : ''}`}
                                </span>
                              </div>
                              {/* 实时拉取明细（后端 detailMsg，如下载层进度） */}
                              {containerActions[container.id].detailMsg && (
                                <span className="text-[10px] text-gray-500 dark:text-gray-400 text-center truncate" title={containerActions[container.id].detailMsg}>
                                  {containerActions[container.id].detailMsg}
                                </span>
                              )}
                            </div>
                          ) : (
                            <>
                              {container.status === 'running' ? (
                                <>
                                  {/* 第一行：停止 / 编辑 / 重启 / 更新 */}
                                  <button
                                    onClick={(e) => { e.stopPropagation(); handleContainerAction(container.id, 'stop') }}
                                    className="flex items-center justify-center gap-1 px-1 py-1.5 text-red-600 dark:text-red-400 bg-white dark:bg-gray-800 hover:bg-red-50 dark:hover:bg-red-900/20 border border-gray-200 dark:border-gray-700 hover:border-red-200 dark:hover:border-red-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                    title="停止"
                                  >
                                    <Square className="h-4 w-4" />
                                    <span>停止</span>
                                  </button>
                                  <button
                                    onClick={(e) => { e.stopPropagation(); setEditTarget({ ...container, ID: container.id }) }}
                                    className="flex items-center justify-center gap-1 px-1 py-1.5 text-orange-600 dark:text-orange-400 bg-white dark:bg-gray-800 hover:bg-orange-50 dark:hover:bg-orange-900/20 border border-gray-200 dark:border-gray-700 hover:border-orange-200 dark:hover:border-orange-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                    title="编辑"
                                  >
                                    <Edit3 className="h-4 w-4" />
                                    <span>编辑</span>
                                  </button>
                                  <button
                                    onClick={(e) => { e.stopPropagation(); handleContainerAction(container.id, 'restart') }}
                                    className="flex items-center justify-center gap-1 px-1 py-1.5 text-blue-600 dark:text-blue-400 bg-white dark:bg-gray-800 hover:bg-blue-50 dark:hover:bg-blue-900/20 border border-gray-200 dark:border-gray-700 hover:border-blue-200 dark:hover:border-blue-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                    title="重启"
                                  >
                                    <RotateCcw className="h-4 w-4" />
                                    <span>重启</span>
                                  </button>
                                </>
                              ) : (
                                <button
                                  onClick={(e) => { e.stopPropagation(); handleContainerAction(container.id, 'start') }}
                                  className="col-span-2 flex items-center justify-center gap-1 px-1 py-1.5 text-green-600 dark:text-green-400 bg-white dark:bg-gray-800 hover:bg-green-50 dark:hover:bg-green-900/20 border border-gray-200 dark:border-gray-700 hover:border-green-200 dark:hover:border-green-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                  title="启动"
                                >
                                  <Play className="h-4 w-4" />
                                  <span>启动</span>
                                </button>
                              )}

                              <button
                                onClick={(e) => { e.stopPropagation(); handleUpdateContainer(container.id) }}
                                className={cn(
                                  "flex items-center justify-center gap-1 px-1 py-1.5 bg-white dark:bg-gray-800 border rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap",
                                  container.haveUpdate
                                    ? "text-yellow-600 dark:text-yellow-400 border-yellow-400 dark:border-yellow-600 hover:bg-yellow-50 dark:hover:bg-yellow-900/20"
                                    : "text-purple-600 dark:text-purple-400 border-gray-200 dark:border-gray-700 hover:bg-purple-50 dark:hover:bg-purple-900/20 hover:border-purple-200 dark:hover:border-purple-800"
                                )}
                                title="更新"
                              >
                                <Upload className="h-4 w-4" />
                                <span>更新</span>
                              </button>
                              {/* 第二行：日志 / 进程 / 控制台 / 文件（进程仅运行中有意义） */}
                              <button
                                onClick={(e) => { e.stopPropagation(); setLogsTarget(container) }}
                                className="flex items-center justify-center gap-1 px-1 py-1.5 text-sky-600 dark:text-sky-400 bg-white dark:bg-gray-800 hover:bg-sky-50 dark:hover:bg-sky-900/20 border border-gray-200 dark:border-gray-700 hover:border-sky-200 dark:hover:border-sky-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                title="查看日志"
                              >
                                <FileText className="h-4 w-4" />
                                <span>日志</span>
                              </button>
                              {container.status === 'running' && (
                                <button
                                  onClick={(e) => { e.stopPropagation(); setProcessTarget({ ...container, ID: container.id }) }}
                                  className="flex items-center justify-center gap-1 px-1 py-1.5 text-emerald-600 dark:text-emerald-400 bg-white dark:bg-gray-800 hover:bg-emerald-50 dark:hover:bg-emerald-900/20 border border-gray-200 dark:border-gray-700 hover:border-emerald-200 dark:hover:border-emerald-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                  title="进程"
                                >
                                  <Activity className="h-4 w-4" />
                                  <span>进程</span>
                                </button>
                              )}
                              <button
                                onClick={(e) => { e.stopPropagation(); setConsoleTarget(container) }}
                                className="flex items-center justify-center gap-1 px-1 py-1.5 text-teal-600 dark:text-teal-400 bg-white dark:bg-gray-800 hover:bg-teal-50 dark:hover:bg-teal-900/20 border border-gray-200 dark:border-gray-700 hover:border-teal-200 dark:hover:border-teal-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                title="终端"
                              >
                                <TerminalSquare className="h-4 w-4" />
                                <span>终端</span>
                              </button>
                              <button
                                onClick={(e) => { e.stopPropagation(); setFileTarget(container) }}
                                className="flex items-center justify-center gap-1 px-1 py-1.5 text-amber-600 dark:text-amber-400 bg-white dark:bg-gray-800 hover:bg-amber-50 dark:hover:bg-amber-900/20 border border-gray-200 dark:border-gray-700 hover:border-amber-200 dark:hover:border-amber-800 rounded-lg transition-all duration-200 shadow-sm hover:shadow active:scale-95 text-xs font-medium whitespace-nowrap"
                                title="文件管理"
                              >
                                <FolderOpen className="h-4 w-4" />
                                <span>文件</span>
                              </button>
                            </>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
          </div>
        ) : (
          <div className="text-center py-12">
            <Package className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900 dark:text-white">暂无容器</h3>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              当前没有运行中的Docker容器
            </p>
          </div>
        )}
      </div>

      {/* 【新】独立的日志弹窗 */}
      {logsTarget && (
        <ContainerLogs
          container={logsTarget}
          onClose={() => setLogsTarget(null)}
        />
      )}
      {/* 【新】独立的控制台弹窗 */}
      {consoleTarget && (
        <ContainerConsole
          container={consoleTarget}
          onClose={() => setConsoleTarget(null)}
        />
      )}
      {/* 文件管理弹窗 */}
      {fileTarget && (
        <FileManager
          container={fileTarget}
          onClose={() => setFileTarget(null)}
        />
      )}

      {/* 创建容器弹窗：创建成功后刷新列表 */}
      {showCreate && (
        <CreateContainerModal
          onClose={() => setShowCreate(false)}
          onCreated={() => {
            // 创建任务已提交，稍后刷新列表查看新容器
            setTimeout(() => refetch(), 1500)
          }}
        />
      )}

      {/* 容器详情弹窗 */}
      {
        selectedContainer && (
          <ContainerDetailModal
            container={selectedContainer}
            stat={getStat(selectedContainer.id)}
            onClose={() => setSelectedContainer(null)}
            onRename={handleRenameContainer}
            onUpdate={handleUpdateContainer}
            onAction={handleContainerAction}
            onEdit={(c) => setEditTarget(c)}
            onProcess={(c) => setProcessTarget(c)}
          />
        )
      }

      {/* 编辑弹窗 */}
      {editTarget && (
        <ContainerEditModal
          container={editTarget}
          onClose={() => setEditTarget(null)}
          onSuccess={() => {
            setEditTarget(null)
            refetch()
          }}
        />
      )}

      {/* 进程弹窗 */}
      {processTarget && (
        <ContainerProcessModal
          container={processTarget}
          onClose={() => setProcessTarget(null)}
        />
      )}
    </div >
  )
}

// 容器详情弹窗组件
function ContainerDetailModal({ container, onClose, onRename, onUpdate, onAction, stat, onEdit, onProcess }) {
  const queryClient = useQueryClient()
  // Tab 分页状态
  const [activeTab, setActiveTab] = useState('basic')
  const [name, setName] = useState(container.name)
  const [imageNameAndTag, setImageNameAndTag] = useState(container.usingImage)
  const [isUpdating, setIsUpdating] = useState(false)
  const [isRenaming, setIsRenaming] = useState(false)
  const [isActionProcessing, setIsActionProcessing] = useState(false)
  const [currentAction, setCurrentAction] = useState('')
  const [currentContainer, setCurrentContainer] = useState(container)
  const fileInputRef = React.useRef(null)
  const [isUploadingIcon, setIsUploadingIcon] = useState(false)
  // 图标操作菜单（上传图片 / 填写URL）是否展开
  const [showIconMenu, setShowIconMenu] = useState(false)
  // 【新】独立的日志和控制台弹窗显示状态
  const [showLogs, setShowLogs] = useState(false)
  const [showConsole, setShowConsole] = useState(false)
  // 文件管理弹窗显示状态
  const [showFileMgr, setShowFileMgr] = useState(false)
  // 容器 Inspect 详情数据（供 网络/挂载/环境变量/资源限制/其他 等 Tab 使用）
  // 列表接口只返回精简字段，完整信息需按需调用 inspect 接口获取
  const [inspectData, setInspectData] = useState(null)
  const [inspectLoading, setInspectLoading] = useState(false)

  // 打开详情弹窗时按需加载 inspect 完整信息，回填各 Tab
  React.useEffect(() => {
    let cancelled = false
    ;(async () => {
      setInspectLoading(true)
      try {
        const r = await containerAPI.inspectContainer(container.id, container.hostId)
        const d = r.data?.data || r.data || null
        if (!cancelled) setInspectData(d)
      } catch (e) {
        console.error('加载容器 inspect 失败:', e)
        if (!cancelled) setInspectData(null)
      } finally {
        if (!cancelled) setInspectLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [container.id])

  // 获取自定义图标配置
  const { data: customIcons = {} } = useQuery({
    queryKey: ['customIcons'],
    queryFn: async () => {
      const response = await imageAPI.getIcons()
      if (response.data.code === 200 || response.data.code === 0) {
        return response.data.data || {}
      }
      return {}
    },
    initialData: () => JSON.parse(localStorage.getItem('docker_copilot_image_logos') || '{}'),
  })

  // 当容器切换时，更新表单字段的值
  React.useEffect(() => {
    setName(container.name)
    setImageNameAndTag(container.usingImage)
    setCurrentContainer(container)
  }, [container])

  // 实时更新容器状态
  React.useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const response = await containerAPI.getContainers();
        if (response.data.code === 0) {
          const containers = response.data.data;
          const updatedContainer = containers.find(c => c.id === container.id);
          if (updatedContainer) {
            // 检查是否有镜像图标
            const imageLogos = JSON.parse(localStorage.getItem('docker_copilot_image_logos') || '{}');

            // 如果容器没有自定义图标，则查找镜像图标
            if (!updatedContainer.iconUrl) {
              // 使用完整的镜像名称和标签进行匹配
              const imageFullName = updatedContainer.usingImage;

              // 首先尝试精确匹配（包含tag）
              if (imageLogos[imageFullName]) {
                updatedContainer.iconUrl = imageLogos[imageFullName];
              } else {
                // 如果精确匹配失败，尝试镜像名称匹配（不包含tag部分）
                const imageName = updatedContainer.usingImage.split(':')[0];

                // 遍历所有镜像图标，查找匹配的镜像名称
                for (const [imageId, logoUrl] of Object.entries(imageLogos)) {
                  // 检查镜像名称是否匹配（不包含tag部分）
                  const logoImageName = imageId.split(':')[0];
                  if (imageName === logoImageName) {
                    updatedContainer.iconUrl = logoUrl;
                    break;
                  }
                }
              }
            }

            setCurrentContainer(updatedContainer);
          }
        }
      } catch (error) {
        console.error('获取容器状态失败:', error);
      }
    }, 3000); // 每3秒获取一次最新状态

    return () => clearInterval(interval);
  }, [container.id]);

  // 图标上传/在线URL/自动获取逻辑已统一抽到 IconEditor 组件

  const handleContainerAction = async (action) => {
    try {
      setIsActionProcessing(true);
      setCurrentAction(action);

      // 调用传入的onAction函数执行实际操作
      if (action === 'update') {
        await onUpdate(container.id);
      } else {
        await onAction(container.id, action);
      }

      // 无效化查询以触发重新获取数据
      await queryClient.invalidateQueries(['containers'])

      setIsActionProcessing(false);
      setCurrentAction('');
    } catch (error) {
      console.error('操作失败:', error);
      setIsActionProcessing(false);
      setCurrentAction('');
    }
  };

  const handleRename = async () => {
    if (name !== currentContainer.name) {
      try {
        setIsRenaming(true)
        console.log(`重命名容器: ${currentContainer.name} -> ${name}`)

        await onRename(container.id, name)

        // 无效化查询以触发重新获取数据
        await queryClient.invalidateQueries(['containers'])

        // 更新当前容器状态
        setCurrentContainer({ ...currentContainer, name: name })
        // 同时更新表单中的名称
        setName(name);

        console.log('✅ 容器重命名成功')
        setIsRenaming(false)
      } catch (error) {
        console.error('重命名失败:', error)
        setIsRenaming(false)
      }
    }
  }

  const handleSave = async () => {
    // 如果镜像tag发生变化，则更新容器
    if (imageNameAndTag !== currentContainer.usingImage) {
      try {
        setIsUpdating(true)

        console.log(`开始更新容器镜像: ${currentContainer.name}`)
        console.log(`原镜像: ${currentContainer.usingImage}`)
        console.log(`新镜像: ${imageNameAndTag}`)

        // 直接调用API更新容器
        const response = await containerAPI.updateContainer(
          container.id,
          container.name,
          imageNameAndTag,
          true, // 删除旧容器
          container.hostId
        )

        console.log('更新容器响应:', response.data)

        if (response.data.code === 200 || response.data.code === 0) {
          const taskID = response.data.data?.taskID

          if (taskID) {
            // 如果返回了taskID，我们需要触发进度轮询
            console.log('更新任务已创建，taskID:', taskID)

            // 关闭弹窗
            onClose()

            // 触发父组件中的进度轮询
            onUpdate(container.id, taskID)

            console.log('✅ 容器更新任务已启动，请在列表中查看进度')
          } else {
            // 没有taskID，更新完成
            await queryClient.invalidateQueries(['containers'])
            setImageNameAndTag(imageNameAndTag) // 更新本地状态
            console.log('✅ 容器镜像更新完成')
          }
        } else {
          throw new Error(response.data.msg || '更新失败')
        }

        setIsUpdating(false)
      } catch (error) {
        console.error('更新容器镜像失败:', error)
        // 增加超时错误的处理
        if (error.code === 'ECONNABORTED' || error.message.includes('timeout')) {
          console.error(`更新操作已提交，但连接超时。请稍后手动刷新页面查看操作结果`)
          // 即使超时也关闭弹窗并触发轮询，因为操作可能仍在进行中
          onClose()
          onUpdate(container.id)
        }
        setIsUpdating(false)
      }
    }
  }





  // 获取状态指示器颜色
  const getStatusIndicatorColor = (status) => {
    const statusConfig = {
      running: 'bg-green-500',
      stopped: 'bg-red-500',
      restarting: 'bg-yellow-500',
      paused: 'bg-blue-500'
    }

    return statusConfig[status?.toLowerCase()] || 'bg-gray-500'
  }

  // 获取容器图标 - 与列表/卡片显示逻辑一致（详情弹窗无 faviconMap，传空对象）
  const getContainerIcon = () => {
    const iconUrl = resolveContainerIcon(currentContainer, {}, customIcons);

    const IconContent = () => {
      if (iconUrl) {
        return (
          <img
            src={iconUrl}
            alt={currentContainer.name}
            className="h-12 w-12 rounded-xl object-cover"
            onError={(e) => {
              e.target.style.display = 'none';
              e.target.nextSibling.style.display = 'flex';
            }}
          />
        );
      }
      return null;
    };

    const FallbackIcon = () => (
      <div className="h-12 w-12 bg-gradient-to-r from-blue-500 to-purple-600 rounded-xl flex items-center justify-center text-white" style={{ display: iconUrl ? 'none' : 'flex' }}>
        <Package className="h-6 w-6" />
      </div>
    );

    return (
      <div className="relative">
        <div
          className="relative group cursor-pointer"
          onClick={() => !isUploadingIcon && setShowIconMenu(true)}
          title="点击设置容器图标"
        >
          <IconContent />
          <FallbackIcon />

          {/* 悬停覆盖层 */}
          <div className="absolute inset-0 bg-black bg-opacity-50 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
            {isUploadingIcon ? (
              <RefreshCw className="h-5 w-5 text-white animate-spin" />
            ) : (
              <Pencil className="h-4 w-4 text-white" />
            )}
          </div>

          {/* 常驻编辑角标：提示图标可点击设置 */}
          {!isUploadingIcon && (
            <div className="absolute -bottom-1 -right-1 h-5 w-5 rounded-full bg-primary-600 text-white flex items-center justify-center shadow ring-2 ring-white dark:ring-gray-800">
              <Pencil className="h-2.5 w-2.5" />
            </div>
          )}
        </div>

        {/* 图标编辑面板：预览 + 上传/在线URL/自动获取 */}
        {showIconMenu && (
          <IconEditor
            imageName={imageNameAndTag || currentContainer.usingImage}
            container={currentContainer}
            currentIconUrl={iconUrl}
            onClose={() => setShowIconMenu(false)}
            onApplied={(url) => {
              setCurrentContainer(prev => ({ ...prev, iconUrl: url }))
              window.dispatchEvent(new Event('storage'))
              queryClient.invalidateQueries(['containers'])
              queryClient.invalidateQueries(['customIcons'])
              setShowIconMenu(false)
            }}
          />
        )}
      </div>
    );
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white dark:bg-gray-800 rounded-2xl shadow-xl w-full max-w-md overflow-hidden">
        {/* 弹窗头部 */}
        <div className="border-b border-gray-200 dark:border-gray-700 px-6 py-4">
          <div className="flex justify-between items-center">
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white">容器详情</h3>
              <div className="flex items-center mt-1">
                {getContainerIcon()}
                {/* 状态指示器竖线 */}
                <div className="flex flex-col items-center justify-center h-full ml-3">
                  <div className={cn(
                    "w-1 h-8 rounded-full",
                    getStatusIndicatorColor(currentContainer.status)
                  )}></div>
                </div>
                <div className="ml-3">
                  <span className="text-sm font-medium text-gray-900 dark:text-white">
                    {currentContainer.name}
                  </span>
                  <div className="flex items-center mt-1">
                    <span className="text-xs text-gray-500 dark:text-gray-400">
                      {(currentContainer.id || '').substring(0, 12)}
                    </span>
                  </div>
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={onClose}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
          </div>
        </div>

        {/* Tab 导航 */}
        <div className="border-b border-gray-200 dark:border-gray-700">
          <nav className="flex px-6 -mb-px space-x-4">
            {[
              { id: 'basic', label: '基本信息' },
              { id: 'network', label: '网络' },
              { id: 'mounts', label: '挂载' },
              { id: 'env', label: '环境变量' },
              { id: 'resources', label: '资源限制' },
              { id: 'other', label: '其他' },
            ].map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  'py-2 px-1 border-b-2 font-medium text-sm transition-colors',
                  activeTab === tab.id
                    ? 'border-primary-600 text-primary-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300'
                )}
              >
                {tab.label}
              </button>
            ))}
          </nav>
        </div>

        {/* 【新】独立的日志弹窗（详情弹窗内打开） */}
        {showLogs && (
          <ContainerLogs
            container={currentContainer}
            onClose={() => setShowLogs(false)}
          />
        )}

        {/* 【新】独立的控制台弹窗（详情弹窗内打开） */}
        {showConsole && (
          <ContainerConsole
            container={currentContainer}
            onClose={() => setShowConsole(false)}
          />
        )}

        {/* 文件管理弹窗（详情弹窗内直接打开） */}
        {showFileMgr && (
          <FileManager
            container={currentContainer}
            onClose={() => setShowFileMgr(false)}
          />
        )}

        {/* 弹窗内容 */}
        <div className="px-6 py-4 space-y-4">
          {/* Tab: 基本信息（现有全部内容） */}
          {activeTab === 'basic' && (
            <>
              {/* 实时资源图表（仅运行中容器） */}
          {currentContainer.status === 'running' && (
            <StatsChart stat={stat} />
          )}

          {/* 容器名称 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              容器名称
            </label>
            <div className="flex space-x-2">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="input flex-1"
                placeholder="输入容器名称"
              />
              <button
                onClick={handleRename}
                disabled={isRenaming || (name === currentContainer.name) || isActionProcessing || isUpdating}
                className={`px-3 py-2 text-sm rounded-lg transition-colors ${isRenaming || (name === currentContainer.name)
                  ? 'bg-gray-200 text-gray-500 cursor-not-allowed dark:bg-gray-700 dark:text-gray-400'
                  : 'bg-primary-600 text-white hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600'
                  }`}
              >
                {isRenaming ? (
                  <>
                    <RefreshCw className="h-4 w-4 mr-1 animate-spin" />
                    重命名中
                  </>
                ) : '重命名'}
              </button>
            </div>
          </div>

          {/* 镜像信息 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              镜像名称和标签
            </label>
            <div className="flex space-x-2">
              <input
                type="text"
                value={imageNameAndTag}
                onChange={(e) => setImageNameAndTag(e.target.value)}
                className="input flex-1"
                placeholder="例如: nginx:latest"
                disabled={isActionProcessing || isUpdating}
              />
              <button
                onClick={handleSave}
                disabled={isUpdating || (imageNameAndTag === currentContainer.usingImage) || !imageNameAndTag.trim()}
                className={`px-3 py-2 text-sm rounded-lg transition-colors flex items-center ${isUpdating || (imageNameAndTag === currentContainer.usingImage) || !imageNameAndTag.trim()
                  ? 'bg-gray-200 text-gray-500 cursor-not-allowed dark:bg-gray-700 dark:text-gray-400'
                  : 'bg-primary-600 text-white hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600'
                  }`}
              >
                {isUpdating ? (
                  <>
                    <RefreshCw className="h-3 w-3 mr-1 animate-spin" />
                    更新中
                  </>
                ) : (
                  '更换镜像'
                )}
              </button>
            </div>
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
              修改镜像后点击"更换镜像"按钮将重新创建容器
            </p>
          </div>
            </>
          )}

          {/* 非基本信息的 Tab 依赖 inspect 数据，加载中时统一提示 */}
          {activeTab !== 'basic' && inspectLoading && (
            <div className="flex items-center justify-center py-8 text-gray-500 text-sm">
              <RefreshCw className="h-5 w-5 animate-spin mr-2" /> 加载详情中...
            </div>
          )}
          {activeTab !== 'basic' && !inspectLoading && !inspectData && (
            <div className="text-center py-8 text-gray-400 text-sm">未能加载容器详情</div>
          )}

          {/* Tab: 网络 */}
          {activeTab === 'network' && !inspectLoading && inspectData && (
            <div className="space-y-3">
              <InfoRow label="网络模式" value={inspectData?.HostConfig?.NetworkMode || '—'} />
              <InfoRow label="IP地址" value={inspectData?.NetworkSettings?.IPAddress || '—'} />
              <InfoRow label="网关" value={inspectData?.NetworkSettings?.Gateway || '—'} />
              <InfoRow label="MAC地址" value={inspectData?.NetworkSettings?.MacAddress || '—'} />
              {/* 自定义网络时逐个列出网络名与 IP */}
              {inspectData?.NetworkSettings?.Networks && Object.entries(inspectData.NetworkSettings.Networks).map(([netName, net]) => (
                <InfoRow key={netName} label={`网络 ${netName}`} value={net?.IPAddress || '—'} />
              ))}
            </div>
          )}

          {/* Tab: 挂载 */}
          {activeTab === 'mounts' && !inspectLoading && inspectData && (
            <div className="space-y-2">
              {(inspectData?.Mounts || []).length === 0 ? (
                <p className="text-gray-500 text-sm">无挂载</p>
              ) : (
                inspectData.Mounts.map((m, i) => (
                  <div key={i} className="p-3 bg-gray-50 dark:bg-gray-900/50 rounded text-sm space-y-1">
                    <div><b>类型:</b> {m.Type}</div>
                    <div><b>源:</b> <code className="text-xs break-all">{m.Source || m.Name}</code></div>
                    <div><b>目标:</b> <code className="text-xs break-all">{m.Destination}</code></div>
                    <div><b>读写:</b> {m.RW ? '读写' : '只读'}</div>
                  </div>
                ))
              )}
            </div>
          )}

          {/* Tab: 环境变量 */}
          {activeTab === 'env' && !inspectLoading && inspectData && (
            <div className="space-y-1 font-mono text-xs">
              {(inspectData?.Config?.Env || []).length === 0 ? (
                <p className="text-gray-500 text-sm font-sans">无环境变量</p>
              ) : (
                (inspectData?.Config?.Env || []).map((line, i) => (
                  <div key={i} className="p-2 bg-gray-50 dark:bg-gray-900/50 rounded break-all">{line}</div>
                ))
              )}
            </div>
          )}

          {/* Tab: 资源限制 */}
          {activeTab === 'resources' && !inspectLoading && inspectData && (
            <div className="space-y-3">
              <InfoRow label="CPU限制" value={inspectData?.HostConfig?.NanoCpus ? `${inspectData.HostConfig.NanoCpus / 1e9} 核` : '无限制'} />
              <InfoRow label="内存限制" value={inspectData?.HostConfig?.Memory ? `${(inspectData.HostConfig.Memory / 1024 / 1024).toFixed(0)} MB` : '无限制'} />
              <InfoRow label="CPU Shares" value={inspectData?.HostConfig?.CpuShares || '默认'} />
              <InfoRow label="重启策略" value={inspectData?.HostConfig?.RestartPolicy?.Name || '—'} />
            </div>
          )}

          {/* Tab: 其他 */}
          {activeTab === 'other' && !inspectLoading && inspectData && (
            <div className="space-y-3">
              <InfoRow label="主机名" value={inspectData?.Config?.Hostname || '—'} />
              <InfoRow label="工作目录" value={inspectData?.Config?.WorkingDir || '/'} />
              <InfoRow label="入口点" value={(inspectData?.Config?.Entrypoint || []).join(' ') || '—'} />
              <InfoRow label="命令" value={(inspectData?.Config?.Cmd || []).join(' ') || '—'} />
            </div>
          )}

        </div>

        {/* 弹窗底部操作按钮 */}
        <div className="border-t border-gray-200 dark:border-gray-700 px-6 py-4 bg-gray-50 dark:bg-gray-700/30">
          <div className="flex flex-wrap items-center justify-between gap-2">

            {/* 操作按钮区：4列×2行 Grid 布局 */}
            <div className="w-full grid grid-cols-4 gap-2">
              {/* 第一行：停止/编辑/重启/更新 */}
              {currentContainer.status === 'running' ? (
                <>
                  <ActionBtn onClick={() => handleContainerAction('stop')} disabled={isActionProcessing || isUpdating}
                    loading={isActionProcessing && currentAction === 'stop'} icon={Square} label="停止" color="red" />
                  <ActionBtn onClick={() => onEdit({ ...currentContainer, ID: currentContainer.id })} disabled={isActionProcessing || isUpdating}
                    icon={Edit3} label="编辑" color="orange" />
                  <ActionBtn onClick={() => handleContainerAction('restart')} disabled={isActionProcessing || isUpdating}
                    loading={isActionProcessing && currentAction === 'restart'} icon={RotateCcw} label="重启" color="yellow" />
                  <ActionBtn onClick={() => onUpdate(container.id)} disabled={isActionProcessing || isUpdating}
                    loading={isActionProcessing && currentAction === 'update'} icon={Upload} label="更新" color="purple" />
                </>
              ) : (
                <div className="col-span-4">
                  <ActionBtn onClick={() => handleContainerAction('start')} disabled={isActionProcessing || isUpdating}
                    loading={isActionProcessing && currentAction === 'start'} icon={Play} label="启动" color="green" fullWidth />
                </div>
              )}

              {/* 第二行：日志/进程/控制台/文件（仅运行中容器显示） */}
              {currentContainer.status === 'running' && (
                <>
                  <ActionBtn onClick={() => setShowLogs(true)} icon={FileText} label="日志" color="sky" />
                  <ActionBtn onClick={() => onProcess({ ...currentContainer, ID: currentContainer.id })} icon={Activity} label="进程" color="emerald" />
                  <ActionBtn onClick={() => setShowConsole(true)} icon={TerminalSquare} label="终端" color="teal" />
                  <ActionBtn onClick={() => setShowFileMgr(true)} icon={FolderOpen} label="文件" color="amber" />
                </>
              )}
            </div>

          </div>
        </div>
      </div>
    </div>
  )
}

// ActionBtn 辅助组件（用于详情弹窗底部按钮）
function ActionBtn({ onClick, disabled, loading, icon: Icon, label, color, fullWidth }) {
  // 与外部容器卡片按钮统一：浅色模式浅白底 + 灰边框，仅用字体/图标颜色区分。
  // 每个颜色包含：文字色 + hover 浅背景 + hover 边框色。
  const colorMap = {
    red: 'text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 hover:border-red-200 dark:hover:border-red-800',
    orange: 'text-orange-600 dark:text-orange-400 hover:bg-orange-50 dark:hover:bg-orange-900/20 hover:border-orange-200 dark:hover:border-orange-800',
    yellow: 'text-yellow-600 dark:text-yellow-400 hover:bg-yellow-50 dark:hover:bg-yellow-900/20 hover:border-yellow-200 dark:hover:border-yellow-800',
    green: 'text-green-600 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20 hover:border-green-200 dark:hover:border-green-800',
    blue: 'text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 hover:border-blue-200 dark:hover:border-blue-800',
    purple: 'text-purple-600 dark:text-purple-400 hover:bg-purple-50 dark:hover:bg-purple-900/20 hover:border-purple-200 dark:hover:border-purple-800',
    sky: 'text-sky-600 dark:text-sky-400 hover:bg-sky-50 dark:hover:bg-sky-900/20 hover:border-sky-200 dark:hover:border-sky-800',
    teal: 'text-teal-600 dark:text-teal-400 hover:bg-teal-50 dark:hover:bg-teal-900/20 hover:border-teal-200 dark:hover:border-teal-800',
    emerald: 'text-emerald-600 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-900/20 hover:border-emerald-200 dark:hover:border-emerald-800',
    amber: 'text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/20 hover:border-amber-200 dark:hover:border-amber-800',
  }
  return (
    <button onClick={onClick} disabled={disabled || loading}
      className={`${fullWidth ? 'w-full' : ''} px-3 py-2 text-sm rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 shadow-sm hover:shadow active:scale-95 transition-all duration-200 flex items-center justify-center gap-1.5 font-medium ${
        disabled || loading ? 'text-gray-400 dark:text-gray-600 cursor-not-allowed' : colorMap[color]
      }`}>
      {loading ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Icon className="h-4 w-4" />}
      <span>{loading ? '处理中' : label}</span>
    </button>
  )
}

// InfoRow 详情弹窗信息行：左标签右值，用于网络/挂载/资源限制/其他等 Tab。
// 此前遗漏定义，导致切换到这些 Tab 时因引用未定义组件而白屏崩溃。
function InfoRow({ label, value }) {
  return (
    <div className="flex items-start justify-between gap-3 py-1.5 border-b border-gray-100 dark:border-gray-700/50 last:border-0">
      <span className="text-sm text-gray-500 dark:text-gray-400 flex-shrink-0">{label}</span>
      <span className="text-sm text-gray-900 dark:text-gray-100 text-right break-all">{value}</span>
    </div>
  )
}

