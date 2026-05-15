// Formatting helpers for human-readable values.

export function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null || bytes < 0) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  const value = bytes / Math.pow(1024, i)
  return `${value.toFixed(value >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function formatNumber(n?: number): string {
  if (n === undefined || n === null) return '—'
  return n.toLocaleString('en-US')
}

export function formatDuration(seconds?: number | null): string {
  if (seconds === undefined || seconds === null) return '—'
  if (seconds < 0) return '—'
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  if (m < 60) return `${m}m ${s}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export function formatLatency(ms?: number): string {
  if (ms === undefined || ms === null || ms < 0) return 'unreachable'
  return `${ms} ms`
}

// stateBadgeColor maps a mongosync state onto a badge color.
export function stateBadgeColor(
  state?: string,
): 'gray' | 'green' | 'blue' | 'yellow' | 'red' {
  switch ((state || '').toUpperCase()) {
    case 'RUNNING':
      return 'green'
    case 'IDLE':
      return 'gray'
    case 'PAUSED':
      return 'yellow'
    case 'COMMITTING':
    case 'COMMITTED':
      return 'blue'
    case 'ERROR':
    case 'FAILED':
      return 'red'
    default:
      return 'gray'
  }
}
