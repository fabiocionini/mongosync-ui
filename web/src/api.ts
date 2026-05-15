// Typed client for the mongosync-ui backend REST API.

import type {
  BinaryStatus,
  LogsResponse,
  MigrationConfig,
  ProgressResponse,
  SessionView,
  StartOptions,
} from './types'

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  let data: any = {}
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = { error: text }
    }
  }
  if (!res.ok) {
    const msg = data?.error || data?.errorDescription || `HTTP ${res.status}`
    throw new Error(msg)
  }
  return data as T
}

export const api = {
  getSession: () => request<SessionView>('GET', '/api/session'),

  binaryStatus: () => request<BinaryStatus>('GET', '/api/binary/status'),
  binaryVersions: () =>
    request<{ versions: string[] }>('GET', '/api/binary/versions'),
  installBinary: (version: string) =>
    request<BinaryStatus>('POST', '/api/binary/install', { version }),

  startLocal: (config: MigrationConfig) =>
    request<SessionView>('POST', '/api/session/local', config),
  attachRemote: (url: string) =>
    request<SessionView>('POST', '/api/session/remote', { url }),
  stopSession: () => request<SessionView>('DELETE', '/api/session'),

  progress: () => request<ProgressResponse>('GET', '/api/progress'),

  // mongosync returns HTTP 200 with {success:false,...} for logical failures,
  // so action results must be checked explicitly, not just by HTTP status.
  start: async (opts: StartOptions) =>
    throwIfFailed(await request<MongosyncResult>('POST', '/api/start', opts)),
  pause: async () =>
    throwIfFailed(await request<MongosyncResult>('POST', '/api/pause')),
  resume: async () =>
    throwIfFailed(await request<MongosyncResult>('POST', '/api/resume')),
  commit: async () =>
    throwIfFailed(await request<MongosyncResult>('POST', '/api/commit')),
  reverse: async () =>
    throwIfFailed(await request<MongosyncResult>('POST', '/api/reverse')),

  logs: () => request<LogsResponse>('GET', '/api/logs'),
}

interface MongosyncResult {
  success?: boolean
  error?: string
  errorDescription?: string
}

// throwIfFailed surfaces mongosync's {success:false} error envelope.
function throwIfFailed(data: MongosyncResult): void {
  if (data && data.success === false) {
    throw new Error(
      data.errorDescription || data.error || 'mongosync rejected the request',
    )
  }
}
