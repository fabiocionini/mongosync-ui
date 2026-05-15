// SessionDetailsView is the read-only record of a completed migration session:
// timeline, connections, outcome and captured logs.

import { useEffect, useState } from 'react'
import { api } from '../api'
import {
  elapsedSeconds,
  formatBytes,
  formatDateTime,
  formatDuration,
  formatNumber,
  formatRatio,
  sessionStatusColor,
} from '../format'
import type { SessionRecord } from '../types'
import { Badge, Banner, Button, Card, Spinner } from './ui'

export function SessionDetailsView({
  id,
  onBack,
}: {
  id: string
  onBack: () => void
}) {
  const [rec, setRec] = useState<SessionRecord | null>(null)
  const [logs, setLogs] = useState<string[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    setLoading(true)
    Promise.all([
      api.session(id),
      api.sessionLogs(id).catch(() => ({ available: false, lines: [] })),
    ])
      .then(([r, l]) => {
        if (!alive) return
        setRec(r)
        setLogs(l.lines)
      })
      .catch((e) => alive && setError(String(e.message || e)))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [id])

  return (
    <div>
      <Button small onClick={onBack}>
        ← All sessions
      </Button>

      {loading && (
        <div className="center-screen">
          <Spinner /> Loading session…
        </div>
      )}

      {!loading && error && (
        <div style={{ marginTop: 16 }}>
          <Banner variant="danger">{error}</Banner>
        </div>
      )}

      {!loading && rec && <Details rec={rec} logs={logs} />}
    </div>
  )
}

function Details({ rec, logs }: { rec: SessionRecord; logs: string[] }) {
  const elapsed = elapsedSeconds(rec.startedAt, rec.endedAt)

  return (
    <div>
      <h1
        className="page-title"
        style={{ display: 'flex', gap: 12, alignItems: 'center', marginTop: 12 }}
      >
        Session details
        <Badge color={sessionStatusColor(rec.status)}>{rec.status}</Badge>
      </h1>
      <p className="page-subtitle" style={{ marginBottom: 4 }}>
        {rec.mode === 'local' ? 'Local migration' : 'Remote attachment'} ·
        started {formatDateTime(rec.startedAt)}
      </p>
      <p className="muted" style={{ fontSize: 12, marginTop: 0, marginBottom: 24 }}>
        Session id: <span className="mono">{rec.id}</span>
      </p>

      <div className="stack">
        {rec.status === 'failed' && rec.outcome && (
          <Banner variant="danger">{rec.outcome}</Banner>
        )}

        <Card title="Timeline">
          <div className="kv">
            <span className="k">Started</span>
            <span className="v">{formatDateTime(rec.startedAt)}</span>
          </div>
          <div className="kv">
            <span className="k">Ended</span>
            <span className="v">
              {rec.endedAt ? formatDateTime(rec.endedAt) : '—'}
            </span>
          </div>
          <div className="kv">
            <span className="k">Elapsed</span>
            <span className="v">{formatDuration(elapsed)}</span>
          </div>
          <div className="kv">
            <span className="k">Final mongosync state</span>
            <span className="v">{rec.lastState || '—'}</span>
          </div>
        </Card>

        <MigrationSummary rec={rec} />

        <Card title="Connection">
          <div className="kv">
            <span className="k">Mode</span>
            <span className="v">{rec.mode}</span>
          </div>
          <div className="kv">
            <span className="k">Source</span>
            <span className="v mono">{rec.source || '—'}</span>
          </div>
          <div className="kv">
            <span className="k">Destination</span>
            <span className="v mono">{rec.destination || '—'}</span>
          </div>
          <div className="kv">
            <span className="k">mongosync API</span>
            <span className="v mono">{rec.apiBaseUrl}</span>
          </div>
          {rec.mongosyncVersion && (
            <div className="kv">
              <span className="k">mongosync version</span>
              <span className="v">{rec.mongosyncVersion}</span>
            </div>
          )}
        </Card>

        <Card
          title="Logs"
          desc={
            rec.mode === 'local'
              ? 'mongosync log captured for this session.'
              : 'Logs are not captured for remote sessions.'
          }
        >
          <div className="logbox">
            {logs.length > 0 ? logs.join('\n') : 'No log output recorded.'}
          </div>
        </Card>
      </div>
    </div>
  )
}

function MigrationSummary({ rec }: { rec: SessionRecord }) {
  const s = rec.summary
  const hasData =
    !!s &&
    (!!s.copiedBytes ||
      !!s.totalBytes ||
      !!s.verifiedDocuments ||
      !!s.estimatedDocuments ||
      !!s.indexesBuilt ||
      !!s.eventsApplied)
  if (!s || !hasData) return null

  return (
    <Card
      title="Migration summary"
      desc="Peak progress observed for this session."
    >
      {s.phase && (
        <div className="kv">
          <span className="k">Final phase</span>
          <span className="v">{s.phase}</span>
        </div>
      )}
      <div className="kv">
        <span className="k">Data copied</span>
        <span className="v">
          {formatBytes(s.copiedBytes)} of {formatBytes(s.totalBytes)}
        </span>
      </div>
      <div className="kv">
        <span className="k">Documents verified</span>
        <span className="v">
          {formatRatio(s.verifiedDocuments, s.estimatedDocuments)}
        </span>
      </div>
      <div className="kv">
        <span className="k">Collections verified</span>
        <span className="v">
          {formatRatio(s.verifiedCollections, s.totalCollections)}
        </span>
      </div>
      <div className="kv">
        <span className="k">Indexes built</span>
        <span className="v">
          {formatNumber(s.indexesBuilt)} / {formatNumber(s.totalIndexes)}
        </span>
      </div>
      <div className="kv">
        <span className="k">Change events applied</span>
        <span className="v">{formatNumber(s.eventsApplied)}</span>
      </div>
    </Card>
  )
}
