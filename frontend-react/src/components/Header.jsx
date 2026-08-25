import React from 'react'
import {
  Box,
  LogOut,
  Server,
  Network,
  DatabaseBackup,
  Info,
  Clock,
  Layers,
  Settings,
  ChevronLeft,
  ChevronRight
} from 'lucide-react'
import { ThemeToggle } from './ThemeToggle.jsx'
import { UpdatePrompt } from './UpdatePrompt.jsx'
import { cn } from '../utils/cn.js'
import logoImg from '../assets/DockerCopilot-logo.png'
import { useVersionCheck } from '../hooks/useVersionCheck.js'

// 全局导航项，桌面侧边栏与移动端底部导航共用，保证一致性
export const NAV_ITEMS = [
  { id: '#containers', label: '容器', icon: Server },
  { id: '#docker-hosts', label: '多 Docker', icon: Network },
  { id: '#images', label: '镜像', icon: Box },
  { id: '#compose', label: '项目', icon: Layers },
  { id: '#schedules', label: '定时', icon: Clock },
  { id: '#backups', label: '备份', icon: DatabaseBackup },
  { id: '#settings', label: '设置', icon: Settings },
  { id: '#about', label: '关于', icon: Info },
]

// 移动端底部导航精简项（空间有限，仅放高频功能）
export const MOBILE_NAV_ITEMS = [
  { id: '#containers', label: '容器', icon: Server },
  { id: '#images', label: '镜像', icon: Box },
  { id: '#compose', label: '项目', icon: Layers },
  { id: '#schedules', label: '定时', icon: Clock },
  { id: '#settings', label: '设置', icon: Settings },
]

export function Sidebar({ activeTab, onTabChange, onLogout, isCollapsed = false, onToggleCollapse, windowWidth = 1024 }) {
  const [isMobileMenuOpen, setIsMobileMenuOpen] = React.useState(false)

  // 时间格式转换函数 - 将UTC时间转换为北京时间
  const formatVersionBuildDate = (dateString) => {
    try {
      const date = new Date(dateString)
      if (isNaN(date.getTime())) {
        return dateString
      }

      // 转换为北京时间 (UTC+8)
      const beijingDate = new Date(date.getTime() + 8 * 60 * 60 * 1000)

      return beijingDate.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      }).replace(/\//g, '-')
    } catch (error) {
      return dateString
    }
  }

  // 使用版本检查 Hook
  const {
    showUpdatePrompt,
    setShowUpdatePrompt,
    backendVersion,
    remoteVersion,
    buildDate,
    hasBackendUpdate,
    updateBackend,
    checkForUpdates
  } = useVersionCheck()

  // 智能判断是否可以手动切换侧边栏
  const canToggleSidebar = windowWidth >= 1024
  const isTabletSize = windowWidth >= 768 && windowWidth < 1024
  const isMobileSize = windowWidth < 768

  const handleToggleCollapse = () => {
    // 只在桌面模式允许切换
    if (canToggleSidebar && onToggleCollapse) {
      onToggleCollapse()
    }
  }

  // 点击侧栏空白区域收起：仅桌面模式且当前为展开态时生效，
  // 收起态不处理（避免误展开）。导航项/按钮已 stopPropagation 不会触发。
  const handleAsideBlankClick = () => {
    if (canToggleSidebar && !isCollapsed && onToggleCollapse) {
      onToggleCollapse()
    }
  }

  const navItems = NAV_ITEMS

  return (
    <>
      {/* 顶部导航栏 - 仅在手机模式（sm）显示 */}
      {windowWidth < 768 && (
        <div className="fixed top-0 left-0 right-0 bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between px-3 sm:px-4 z-40 shadow-sm" style={{ paddingTop: 'env(safe-area-inset-top)', paddingBottom: '0.875rem', height: 'calc(3.5rem + env(safe-area-inset-top))' }}>
          {/* 左侧：Logo 和项目信息 */}
          <button
            onClick={() => setIsMobileMenuOpen(true)}
            className="flex items-center gap-2 hover:opacity-80 transition-opacity active:scale-95 rounded-lg group"
            title="打开菜单"
          >
            <img
              src={logoImg}
              alt="菜单"
              className="h-6 w-6 sm:h-7 sm:w-7 rounded-lg object-cover border-0"
            />
            <div className="flex items-center gap-1">
              <span className="text-sm font-semibold text-gray-900 dark:text-white">Docker Copilot</span>
              <span className="text-xs text-gray-500 dark:text-gray-400">{backendVersion || 'v1.0'}</span>
            </div>
          </button>

          {/* 右侧：主题切换和退出登录 */}
          <div className="flex items-center gap-1">
            <ThemeToggle collapsed={false} />
            <button
              onClick={onLogout}
              className="p-2 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors active:scale-95"
              title="退出登录"
            >
              <LogOut className="h-5 w-5" />
            </button>
          </div>
        </div>
      )}

      {/* 添加顶部导航栏的占位符 - 仅在手机模式显示 */}
      {windowWidth < 768 && <div style={{ height: 'calc(3.5rem + env(safe-area-inset-top))' }} />}

      {/* 侧边栏遮罩 - 仅在手机菜单打开时显示 */}
      {windowWidth < 768 && isMobileMenuOpen && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 z-40"
          onClick={() => setIsMobileMenuOpen(false)}
        />
      )}

      {/* 侧边栏 */}
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 bg-white dark:bg-gray-800 shadow-xl lg:shadow-none transform transition-all duration-300 ease-in-out flex flex-col",
          isCollapsed ? "w-20" : "w-64 sm:w-72 md:w-64",
          // 手机模式：根据菜单打开状态显示/隐藏；md及以上：始终显示
          windowWidth < 768
            ? (isMobileMenuOpen ? "translate-x-0" : "-translate-x-full")
            : "translate-x-0",
          "max-h-screen overflow-y-auto",
          // 手机模式距顶部导航栏下方，其他模式从顶部开始
          windowWidth < 768 ? "top-14" : "top-0",
          "border-r border-gray-200 dark:border-gray-700"
        )}
        onClick={handleAsideBlankClick}
      >
        <div className="flex flex-col h-full">
          {/* 头部 - 现代卡片设计 (仅在非手机模式显示) */}
          {isMobileSize === false && (
            <div className="p-4 sm:p-5 flex-shrink-0">
              <button
                onClick={(e) => { e.stopPropagation(); handleToggleCollapse() }}
                disabled={!canToggleSidebar}
                className={cn(
                  "w-full flex items-center transition-all duration-300 group",
                  !canToggleSidebar ? "cursor-not-allowed opacity-60" : "cursor-pointer hover:opacity-80",
                  isCollapsed ? "justify-center" : "space-x-3"
                )}
                title={
                  isMobileSize ? "手机模式" :
                    isTabletSize ? "平板模式（自动收缩）" :
                      isCollapsed ? "展开侧边栏" : "收起侧边栏"
                }
              >
                <div className="flex-shrink-0">
                  <img
                    src={logoImg}
                    alt="Docker Copilot"
                    className="h-9 w-9 sm:h-11 sm:w-11 rounded-xl object-cover shadow-md group-hover:shadow-lg group-hover:scale-110 transition-all duration-200 border-0"
                  />
                </div>
                {!isCollapsed && isMobileSize === false && (
                  <div className="text-left transition-all duration-300 min-w-0 flex-1">
                    <h1 className="text-base sm:text-lg font-bold text-gray-900 dark:text-white">Docker Copilot</h1>
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">容器管理平台</p>
                  </div>
                )}
              </button>
            </div>
          )}

          {/* 分割线 */}
          <div className="px-4 sm:px-5">
            <div className="h-px bg-gradient-to-r from-transparent via-gray-200 dark:via-gray-700 to-transparent" />
          </div>

          {/* 导航菜单 */}
          <nav className={cn("flex-1 py-6 sm:py-8 overflow-y-auto space-y-2", isCollapsed ? "px-2.5" : "px-3 sm:px-4")}>
            <ul className="space-y-0.5">
              {navItems.map((item, index) => {
                const Icon = item.icon
                const isActive = activeTab === item.id
                return (
                  <li key={item.id}>
                    <button
                      onClick={(e) => {
                        // 阻止冒泡：点击导航项只切换页面，不触发侧栏收起
                        e.stopPropagation()
                        onTabChange(item.id)
                        setIsMobileMenuOpen(false)
                      }}
                      className={cn(
                        "w-full flex items-center rounded-lg text-left transition-all duration-200 group active:scale-95 relative overflow-hidden",
                        isCollapsed ? "justify-center p-2.5 sm:p-3" : "space-x-3 px-3 sm:px-4 py-2.5 sm:py-3",
                        isActive
                          ? "bg-gradient-to-r from-primary-50 to-primary-100/50 dark:from-primary-900/30 dark:to-primary-900/10 text-primary-700 dark:text-primary-300 font-semibold shadow-sm"
                          : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/50"
                      )}
                      title={isCollapsed ? item.label : undefined}
                    >
                      {/* 左侧指示条 */}
                      {isActive && !isCollapsed && (
                        <div className="absolute left-0 top-0 bottom-0 w-1 bg-primary-500 dark:bg-primary-400 rounded-r-full" />
                      )}

                      <Icon className={cn(
                        "flex-shrink-0 transition-all duration-200",
                        isCollapsed ? "h-6 w-6" : "h-5 w-5",
                        isActive && "scale-110"
                      )} />
                      {!isCollapsed && (
                        <span className="truncate text-sm sm:text-base font-medium">{item.label}</span>
                      )}
                    </button>
                  </li>
                )
              })}
            </ul>
          </nav>

          {/* 底部操作区 - 所有尺寸都显示 */}
          <div className={cn("flex flex-col flex-shrink-0", isCollapsed ? "p-2.5" : "p-4 sm:p-5")}>
            {/* 分割线 */}
            <div className="mb-4">
              <div className="h-px bg-gradient-to-r from-transparent via-gray-200 dark:via-gray-700 to-transparent" />
            </div>

            {/* 操作按钮：展开态每个按钮独占整行、左对齐，避免 flex-1 平分造成的挤压 */}
            <div
              className={cn(
                "mb-4 flex flex-col gap-2",
                isCollapsed && "items-center"
              )}
              onClick={(e) => e.stopPropagation()}
            >
              {/* 主题切换 */}
              <div className={cn(isCollapsed ? "" : "w-full")}>
                <ThemeToggle collapsed={isCollapsed} />
              </div>

              {/* 收起/展开按钮 - 仅桌面模式显示 */}
              {windowWidth >= 768 && (
                <button
                  onClick={handleToggleCollapse}
                  className={cn(
                    "flex items-center transition-all duration-200 group active:scale-95 whitespace-nowrap text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg",
                    isCollapsed
                      ? "justify-center p-2.5 w-full"
                      : "gap-2 px-3 py-2.5 w-full"
                  )}
                  title={isCollapsed ? "展开菜单" : "收起菜单"}
                >
                  {isCollapsed ? (
                    <ChevronRight className="h-4 w-4 flex-shrink-0" />
                  ) : (
                    <>
                      <ChevronLeft className="h-4 w-4 flex-shrink-0" />
                      <span className="text-xs sm:text-sm font-medium">收起菜单</span>
                    </>
                  )}
                </button>
              )}

              {/* 退出登录按钮 - 独占整行 */}
              <button
                onClick={onLogout}
                className={cn(
                  "flex items-center transition-all duration-200 group active:scale-95 whitespace-nowrap text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg",
                  isCollapsed
                    ? "justify-center p-2.5 w-full"
                    : "gap-2 px-3 py-2.5 w-full"
                )}
                title={isCollapsed ? "退出登录" : ""}
              >
                <LogOut className="h-4 w-4 flex-shrink-0 group-hover:rotate-12 transition-transform duration-300" />
                {!isCollapsed && (
                  <span className="text-xs sm:text-sm font-medium">退出登录</span>
                )}
              </button>
            </div>

            {/* 版本信息部分 */}
            {isCollapsed ? (
              // 收起状态 - 简约指示
              <div className="space-y-3">
                {/* 状态指示 */}
                <div className="flex justify-center">
                  <span className="w-3 h-3 rounded-full bg-emerald-500 shadow-sm shadow-emerald-400/50 dark:shadow-emerald-600/50" title={`运行中 - ${backendVersion || 'v1.0'}`} />
                </div>

                {/* 开发人员 - 首字母 */}
                <div className="flex justify-center">
                  <div className="flex -space-x-1.5">
                    <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-blue-100 dark:bg-blue-900/40 text-[10px] font-semibold text-blue-600 dark:text-blue-400 ring-2 ring-white dark:ring-gray-800">D</span>
                    <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-purple-100 dark:bg-purple-900/40 text-[10px] font-semibold text-purple-600 dark:text-purple-400 ring-2 ring-white dark:ring-gray-800">O</span>
                  </div>
                </div>

                {/* 更新提示 */}
                {hasBackendUpdate && (
                  <div className="flex justify-center">
                    <span className="w-2.5 h-2.5 rounded-full bg-amber-400 shadow-sm shadow-amber-400/50" title="有新版本" />
                  </div>
                )}
              </div>
            ) : (
              // 展开状态 - 简洁卡片
              <div className="space-y-2.5">
                {/* 版本状态行 */}
                <div className="flex items-center justify-between px-1">
                  <div className="flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                    <span className="text-xs font-medium text-gray-500 dark:text-gray-400">运行中</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <span className="text-[10px] text-gray-400 dark:text-gray-500">v</span>
                    <span className="text-sm font-semibold text-gray-900 dark:text-white">
                      {backendVersion || '1.0'}
                    </span>
                  </div>
                </div>

                {/* 团队信息 - 原作者 + Fork 维护者 */}
                <div className="grid grid-cols-2 gap-2">
                  <div className="rounded-lg bg-gray-50 dark:bg-gray-800/60 px-2.5 py-2">
                    <span className="text-[10px] text-gray-400 dark:text-gray-500">原作者</span>
                    <p className="text-xs font-medium text-gray-800 dark:text-gray-200 mt-0.5">onlyLTY · DongShu</p>
                  </div>
                  <div className="rounded-lg bg-gray-50 dark:bg-gray-800/60 px-2.5 py-2">
                    <span className="text-[10px] text-gray-400 dark:text-gray-500">Fork 维护</span>
                    <p className="text-xs font-medium text-gray-800 dark:text-gray-200 mt-0.5">l429609201</p>
                  </div>
                </div>

                {/* 构建日期 + 更新提示 */}
                <div className="flex items-center justify-between px-1">
                  {buildDate ? (
                    <span className="text-[10px] text-gray-400 dark:text-gray-500" title={formatVersionBuildDate(buildDate)}>
                      构建 {formatVersionBuildDate(buildDate).split(' ')[0]}
                    </span>
                  ) : (
                    <span />
                  )}
                  {hasBackendUpdate && (
                    <button
                      onClick={() => setShowUpdatePrompt(true)}
                      className="text-[10px] font-medium text-amber-600 dark:text-amber-400 hover:text-amber-700 dark:hover:text-amber-300 transition-colors"
                    >
                      有新版本 →
                    </button>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </aside>



      {/* 版本更新提示弹窗 */}
      <UpdatePrompt
        isVisible={showUpdatePrompt}
        onClose={() => setShowUpdatePrompt(false)}
        backendVersion={backendVersion}
        remoteVersion={remoteVersion}
        hasBackendUpdate={hasBackendUpdate}
        onUpdateBackend={updateBackend}
      />
    </>
  )
}

// 手机底部导航栏组件
export function MobileBottomNav({ activeTab, onTabChange, windowWidth = 1024 }) {
  const navItems = MOBILE_NAV_ITEMS

  return (
    <>
      {windowWidth < 768 && (
        <nav 
          className="fixed left-1/2 -translate-x-1/2 w-[calc(100%-1.5rem)] max-w-sm bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-full z-40 shadow-lg transition-all duration-300"
          style={{ 
            bottom: 'calc(env(safe-area-inset-bottom, 0px) + 0.5rem)'
          }}
        >
          <div className="flex items-center justify-around px-2 py-3 gap-0.5">
            {navItems.map((item) => {
              const Icon = item.icon
              const isActive = activeTab === item.id
              return (
                <button
                  key={item.id}
                  onClick={() => onTabChange(item.id)}
                  className={cn(
                    "flex flex-col items-center justify-center gap-1 py-2 px-1.5 rounded-full transition-all duration-200 active:scale-95 flex-1 min-w-0",
                    isActive
                      ? "text-primary-600 dark:text-primary-400 bg-primary-100 dark:bg-primary-900/40"
                      : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700/50"
                  )}
                  title={item.label}
                >
                  <Icon className="h-5 w-5 flex-shrink-0" />
                  <span className="text-xs font-medium">{item.label}</span>
                </button>
              )
            })}
          </div>
        </nav>
      )}
    </>
  )
}