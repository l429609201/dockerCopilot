import React, { useState, useEffect, useCallback } from 'react'
import { X, Folder, File, Plus, FolderPlus, Edit2, Trash2, ArrowUp, RefreshCw, AlertCircle, FileText, Save } from 'lucide-react'
import { composeAPI, apiClient } from '../api/client.js'

// Compose 文件管理器弹窗：浏览 /compose 目录，创建文件夹和 docker-compose.yml 文件
export function ComposeFileManager({ onClose, onFileCreated }) {
  const [current, setCurrent] = useState('/compose')
  const [parent, setParent] = useState('')
  const [dirs, setDirs] = useState([])
  const [files, setFiles] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showCreateFolder, setShowCreateFolder] = useState(false)
  const [showCreateFile, setShowCreateFile] = useState(false)
  const [newFolderName, setNewFolderName] = useState('')
  const [newFileName, setNewFileName] = useState('docker-compose.yml')
  const [creating, setCreating] = useState(false)

  // 文件编辑相关
  const [editingFile, setEditingFile] = useState(null) // { name, content }
  const [fileContent, setFileContent] = useState('')
  const [loadingFile, setLoadingFile] = useState(false)
  const [savingFile, setSavingFile] = useState(false)
  const [validating, setValidating] = useState(false)
  const [validationMsg, setValidationMsg] = useState('')
  const [warnings, setWarnings] = useState([])

  // 加载目录内容
  const load = useCallback(async (path) => {
    setLoading(true)
    setError('')
    try {
      const r = await composeAPI.browse(path || '/compose')
      const d = r.data?.data || {}
      if (r.data?.code !== 200) {
        setError(r.data?.msg || '加载失败')
        return
      }
      setCurrent(d.path || '/compose')
      setParent(d.parent || '')
      setDirs(Array.isArray(d.dirs) ? d.dirs : [])
      setFiles(Array.isArray(d.files) ? d.files : [])
    } catch (e) {
      setError('读取目录失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load('/compose') }, [load])

  // 创建文件夹
  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) {
      alert('请输入文件夹名称')
      return
    }
    setCreating(true)
    try {
      const r = await composeAPI.createFolder(current, newFolderName)
      if (r.data?.code === 200) {
        setShowCreateFolder(false)
        setNewFolderName('')
        load(current) // 刷新当前目录
      } else {
        alert('创建失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      alert('创建失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setCreating(false)
    }
  }

  // 创建 Compose 文件
  const handleCreateFile = async () => {
    if (!newFileName.trim()) {
      alert('请输入文件名')
      return
    }
    setCreating(true)
    try {
      const r = await composeAPI.createFile(current, newFileName)
      if (r.data?.code === 200) {
        setShowCreateFile(false)
        setNewFileName('docker-compose.yml')
        load(current) // 刷新当前目录
        if (onFileCreated) {
          const filePath = r.data?.data?.path || `${current}/${newFileName}`
          onFileCreated(filePath)
        }
      } else {
        alert('创建失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      alert('创建失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setCreating(false)
    }
  }

  // 编辑器部分功能
  const handleEditFile = async (fileName) => {
    setLoadingFile(true)
    setValidationMsg('')
    setWarnings([])
    try {
      const filePath = `${current}/${fileName}`
      // 使用新的通用文件读取 API
      const r = await apiClient.get('/api/files', { params: { path: filePath } })

      if (r.data?.code === 200) {
        setFileContent(r.data.data?.content || '')
        setEditingFile({ name: fileName, path: filePath })
      } else {
        alert('读取文件失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      alert('读取文件失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setLoadingFile(false)
    }
  }

  // 校验 Compose 文件格式
  const handleValidate = async () => {
    if (!fileContent.trim()) {
      setValidationMsg('内容为空')
      return
    }
    setValidating(true)
    setValidationMsg('')
    setWarnings([])
    try {
      const r = await composeAPI.validate(fileContent)
      const d = r.data?.data
      if (d?.valid) {
        setWarnings(d.warnings || [])
        setValidationMsg(d.warnings?.length ? '✅ 语法正确，但有风险提示' : '✅ 校验通过')
      } else {
        setValidationMsg('❌ 校验失败：' + (d?.error || '未知'))
      }
    } catch (e) {
      setValidationMsg('❌ 校验失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setValidating(false)
    }
  }

  // 保存文件
  const handleSaveFile = async () => {
    if (!editingFile) return
    setSavingFile(true)
    setValidationMsg('')
    try {
      // 使用新的通用文件保存 API
      const r = await apiClient.put('/api/files', {
        path: editingFile.path,
        content: fileContent
      })

      if (r.data?.code === 200) {
        setValidationMsg('✅ 保存成功')
        // 刷新目录列表
        load(current)
      } else {
        alert('保存失败：' + (r.data?.msg || '未知错误'))
      }
    } catch (e) {
      alert('保存失败：' + (e.response?.data?.msg || e.message))
    } finally {
      setSavingFile(false)
    }
  }

  // 关闭编辑器
  const handleCloseEditor = () => {
    setEditingFile(null)
    setFileContent('')
    setValidationMsg('')
    setWarnings([])
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-4xl max-h-[85vh] flex flex-col">
        {/* 头部 */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
          <div>
            <h3 className="text-base font-semibold text-gray-900 dark:text-white flex items-center gap-2">
              <Folder className="h-5 w-5 text-primary-600" /> Compose 文件管理器
            </h3>
            <p className="text-xs text-gray-500 mt-1">浏览 /compose 目录，创建项目文件</p>
          </div>
          <button onClick={onClose} className="p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 工具栏 */}
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-700/50 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <button
              onClick={() => load(parent || '/compose')}
              disabled={current === '/compose' || loading}
              className="p-1.5 rounded-lg text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed"
              title="上一级"
            >
              <ArrowUp className="h-4 w-4" />
            </button>
            <code className="flex-1 text-xs bg-gray-50 dark:bg-gray-900 rounded px-2 py-1.5 break-all text-gray-700 dark:text-gray-300">
              {current}
            </code>
            <button onClick={() => load(current)} disabled={loading}
              className="p-1.5 rounded-lg text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-40" title="刷新">
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            </button>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => setShowCreateFolder(true)}
              className="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600">
              <FolderPlus className="h-4 w-4" /> 新建文件夹
            </button>
            <button onClick={() => setShowCreateFile(true)}
              className="flex items-center gap-1 px-3 py-1.5 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700">
              <Plus className="h-4 w-4" /> 新建 Compose 文件
            </button>
          </div>
        </div>

        {/* 文件列表 */}
        <div className="flex-1 overflow-y-auto p-4 min-h-[300px]">
          {error && (
            <div className="m-2 p-3 flex items-start gap-2 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-300 rounded-lg text-sm">
              <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" /><span>{error}</span>
            </div>
          )}

          {loading && dirs.length === 0 && files.length === 0 && (
            <div className="text-center py-10 text-gray-400 text-sm">
              <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2" /> 加载中...
            </div>
          )}

          {!loading && !error && dirs.length === 0 && files.length === 0 && (
            <div className="text-center py-10 text-gray-400 text-sm">该目录下没有文件或文件夹</div>
          )}

          {/* 文件夹列表 */}
          <div className="space-y-1">
            {dirs.map((dir) => (
              <button
                key={dir}
                onClick={() => load(current === '/' ? `/${dir}` : `${current}/${dir}`)}
                className="w-full flex items-center gap-2 px-3 py-2 text-left rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700/50 text-sm text-gray-700 dark:text-gray-200"
              >
                <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
                <span className="truncate">{dir}</span>
              </button>
            ))}

            {/* 文件列表 */}
            {files.map((file) => (
              <div
                key={file}
                className="w-full flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700/30 text-sm text-gray-700 dark:text-gray-200"
              >
                <FileText className="h-4 w-4 text-blue-500 flex-shrink-0" />
                <span className="truncate flex-1">{file}</span>
                <button
                  onClick={() => handleEditFile(file)}
                  className="p-1 text-gray-400 hover:text-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 rounded"
                  title="编辑"
                >
                  <Edit2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* 新建文件夹对话框 */}
        {showCreateFolder && (
          <div className="absolute inset-0 bg-black/50 flex items-center justify-center z-10">
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 w-full max-w-md mx-4">
              <h4 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">新建文件夹</h4>
              <input
                type="text"
                value={newFolderName}
                onChange={(e) => setNewFolderName(e.target.value)}
                placeholder="请输入文件夹名称"
                className="input w-full mb-4"
                autoFocus
              />
              <div className="flex justify-end gap-2">
                <button
                  onClick={() => { setShowCreateFolder(false); setNewFolderName('') }}
                  className="px-4 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg"
                >
                  取消
                </button>
                <button
                  onClick={handleCreateFolder}
                  disabled={creating || !newFolderName.trim()}
                  className="px-4 py-2 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
                >
                  {creating ? '创建中...' : '创建'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 新建 Compose 文件对话框 */}
        {showCreateFile && (
          <div className="absolute inset-0 bg-black/50 flex items-center justify-center z-10">
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 w-full max-w-md mx-4">
              <h4 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">新建 Compose 文件</h4>
              <div className="mb-4">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">文件名</label>
                <input
                  type="text"
                  value={newFileName}
                  onChange={(e) => setNewFileName(e.target.value)}
                  placeholder="docker-compose.yml"
                  className="input w-full"
                  autoFocus
                />
                <p className="text-xs text-gray-500 mt-1">推荐使用 docker-compose.yml 或 compose.yaml</p>
              </div>
              <div className="flex justify-end gap-2">
                <button
                  onClick={() => { setShowCreateFile(false); setNewFileName('docker-compose.yml') }}
                  className="px-4 py-2 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg"
                >
                  取消
                </button>
                <button
                  onClick={handleCreateFile}
                  disabled={creating || !newFileName.trim()}
                  className="px-4 py-2 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
                >
                  {creating ? '创建中...' : '创建'}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* 文件编辑器弹窗 */}
      {editingFile && (
        <div className="absolute inset-0 bg-black/50 flex items-center justify-center z-20">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-4xl mx-4 max-h-[90vh] flex flex-col">
            {/* 编辑器头部 */}
            <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
              <h4 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
                <FileText className="h-5 w-5 text-blue-500" />
                编辑文件：{editingFile.name}
              </h4>
              <button onClick={handleCloseEditor} className="p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* 文件内容编辑区 */}
            <div className="flex-1 p-4 overflow-hidden flex flex-col">
              {loadingFile ? (
                <div className="text-center py-10 text-gray-400">
                  <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2" /> 加载中...
                </div>
              ) : (
                <>
                  <textarea
                    value={fileContent}
                    onChange={(e) => setFileContent(e.target.value)}
                    className="flex-1 min-h-[400px] font-mono text-sm p-3 border border-gray-300 dark:border-gray-600 rounded-lg bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100 resize-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                    spellCheck={false}
                    placeholder="在此编辑 Compose 配置..."
                  />

                  {/* 警告信息 */}
                  {warnings.length > 0 && (
                    <div className="mt-3 p-3 bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 rounded-lg text-xs space-y-1">
                      {warnings.map((w, i) => <div key={i}>⚠️ {w}</div>)}
                    </div>
                  )}

                  {/* 校验结果消息 */}
                  {validationMsg && (
                    <div className={`mt-2 p-2 rounded-lg text-sm ${
                      validationMsg.startsWith('✅')
                        ? 'bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300'
                        : 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300'
                    }`}>
                      {validationMsg}
                    </div>
                  )}
                </>
              )}
            </div>

            {/* 编辑器底部按钮 */}
            <div className="flex items-center justify-between p-4 border-t border-gray-200 dark:border-gray-700">
              <div className="text-sm text-gray-500">
                提示：修改后请先校验，确保格式正确
              </div>
              <div className="flex gap-2">
                <button
                  onClick={handleValidate}
                  disabled={validating || loadingFile}
                  className="px-4 py-2 text-sm bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
                >
                  {validating ? '校验中...' : '🔍 校验格式'}
                </button>
                <button
                  onClick={handleSaveFile}
                  disabled={savingFile || loadingFile}
                  className="px-4 py-2 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 flex items-center gap-1"
                >
                  <Save className="h-4 w-4" />
                  {savingFile ? '保存中...' : '保存'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

