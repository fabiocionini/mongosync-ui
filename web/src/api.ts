// Typed client for the mongosync-ui backend REST API.

import type {
  BinaryStatus,
  LogsResponse,
  MigrationConfig,
  ProgressResponse,
  SessionRecord,
  SessionResponse,
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

export const api = {
  // active session + binary state
  getSession: () => request<SessionResponse>('GET', '/api/session'),

  // session registry / history
  sessions: () =>
    request<{ records: SessionRecord[] }>('GET', '/api/sessions'),
  session: (id: string) =>
    request<SessionRecord>('GET', `/api/sessions/${encodeURIComponent(id)}`),
  sessionLogs: (id: string) =>
    request<LogsResponse>(
      'GET',
      `/api/sessions/${encodeURIComponent(id)}/logs`,
    ),
  deleteSession: (id: string) =>
    request<void>('DELETE', `/api/sessions/${encodeURIComponent(id)}`),

  // binary management
  binaryStatus: () => request<BinaryStatus>('GET', '/api/binary/status'),
  binaryVersions: () =>
    request<{ versions: string[] }>('GET', '/api/binary/versions'),
  installBinary: (version: string) =>
    request<BinaryStatus>('POST', '/api/binary/install', { version }),

  // session lifecycle
  startLocal: (config: MigrationConfig) =>
    request<SessionResponse>('POST', '/api/session/local', config),
  attachRemote: (url: string) =>
    request<SessionResponse>('POST', '/api/session/remote', { url }),
  stopSession: () => request<SessionResponse>('DELETE', '/api/session'),

  // mongosync control (acts on the active session)
  progress: () => request<ProgressResponse>('GET', '/api/progress'),
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
}
