import React from 'react'
import { DataBackups } from './DataBackups.jsx'

// 备份页只保留实际数据归档，不再请求或展示旧配置快照。
export function Backups() {
  return <DataBackups />
}
