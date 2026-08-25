import React, { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

import { cn } from '../utils/cn.js'

// 交互式终端：用 xterm.js + WebSocket 连接容器 exec，实现类似 Portainer 的控制台。
export function Terminal({ containerId, cmd, user, fullscreen = false, hostId }) {
  const containerRef = useRef(null)
  const termRef = useRef(null)
  const wsRef = useRef(null)
  const fitRef = useRef(null) // 暴露 fit 方法给全屏切换时调用

  useEffect(() => {
    if (!containerRef.current) return

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, Consolas, monospace',
      theme: { background: '#1e1e1e', foreground: '#e0e0e0' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current)
    fit.fit()
    termRef.current = term

    // 组装 WebSocket 地址（复用当前 host + JWT token）
    const token = localStorage.getItem('docker_copilot_token') || ''
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const base = `${proto}://${window.location.host}`
    const params = new URLSearchParams({ cmd: cmd || '/bin/sh', user: user || '', token })
    // 多 Docker 管理：带上容器所属主机，后端据此选择对应 client
    if (hostId) params.set('hostId', hostId)
    const ws = new WebSocket(`${base}/api/container/${containerId}/exec/ws?${params.toString()}`)
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onopen = () => {
      term.writeln('\x1b[32m已连接到容器终端\x1b[0m')
      sendResize()
    }
    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        term.write(ev.data)
      } else {
        term.write(new Uint8Array(ev.data))
      }
    }
    ws.onclose = () => term.writeln('\r\n\x1b[33m连接已关闭\x1b[0m')
    ws.onerror = () => term.writeln('\r\n\x1b[31m连接出错\x1b[0m')

    // 键盘输入 -> 后端
    const dataDisp = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data)
    })

    // 尺寸调整 -> 重新 fit 并发送 resize 控制消息
    function sendResize() {
      try { fit.fit() } catch { /* 容器未就绪时忽略 */ }
      const { cols, rows } = term
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    }
    fitRef.current = sendResize
    const onResize = () => sendResize()
    window.addEventListener('resize', onResize)

    // 监听容器自身尺寸变化（如弹窗全屏切换），比只听 window 更准确
    const ro = new ResizeObserver(() => sendResize())
    ro.observe(containerRef.current)

    return () => {
      window.removeEventListener('resize', onResize)
      ro.disconnect()
      fitRef.current = null
      dataDisp.dispose()
      try { ws.close() } catch { /* ignore */ }
      term.dispose()
    }
  }, [containerId, cmd, user, hostId])

  // 全屏状态切换后，等布局稳定再 fit 一次，确保终端填满新区域
  useEffect(() => {
    const t = setTimeout(() => fitRef.current?.(), 60)
    return () => clearTimeout(t)
  }, [fullscreen])

  return (
    <div
      ref={containerRef}
      className={cn(
        'w-full bg-[#1e1e1e] rounded-lg overflow-hidden',
        fullscreen ? 'flex-1 min-h-0' : 'h-[400px]',
      )}
    />
  )
}
