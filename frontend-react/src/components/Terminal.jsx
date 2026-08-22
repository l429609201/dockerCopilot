import React, { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

// 交互式终端：用 xterm.js + WebSocket 连接容器 exec，实现类似 Portainer 的控制台。
export function Terminal({ containerId, cmd, user }) {
  const containerRef = useRef(null)
  const termRef = useRef(null)
  const wsRef = useRef(null)

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

    // 尺寸调整 -> 发送 resize 控制消息
    function sendResize() {
      fit.fit()
      const { cols, rows } = term
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    }
    const onResize = () => sendResize()
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      dataDisp.dispose()
      try { ws.close() } catch { /* ignore */ }
      term.dispose()
    }
  }, [containerId, cmd, user])

  return <div ref={containerRef} className="w-full h-[400px] bg-[#1e1e1e] rounded-lg overflow-hidden" />
}
