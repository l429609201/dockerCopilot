// 格式化运行时间为中文
// 支持已中文化直接返回，或解析英文格式（"2h 30m"、"1 day 2h" 等）
export function formatRunningTime(runningTime) {
  if (!runningTime) return '未知'

  if (runningTime.includes('小时') || runningTime.includes('分钟') || runningTime.includes('秒')) {
    return runningTime
  }

  let hours = 0
  let minutes = 0
  let days = 0

  const dayMatch = runningTime.match(/(\d+)\s*(?:day|d)/)
  if (dayMatch) days = parseInt(dayMatch[1])

  const hourMatch = runningTime.match(/(\d+)\s*(?:hour|h)/)
  if (hourMatch) hours = parseInt(hourMatch[1])

  const minMatch = runningTime.match(/(\d+)\s*(?:minute|min|m)/)
  if (minMatch) minutes = parseInt(minMatch[1])

  let result = ''
  if (days > 0) result += `${days}天 `
  if (hours > 0) result += `${hours}小时 `
  if (minutes > 0 || (days === 0 && hours === 0)) result += `${minutes}分钟`

  return result.trim()
}
