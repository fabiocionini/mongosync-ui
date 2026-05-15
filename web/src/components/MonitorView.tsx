// MonitorView is the live dashboard for an active migration session: progress,
// metrics, verification, warnings, logs and the lifecycle action controls.

import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import {
  formatBytes,
  formatDuration,
  formatLatency,
  formatNumber,
  stateBadgeColor,
} from '../format'
import type {
  Namespace,
  Progress,
  ProgressResponse,
  SessionView,
  StartOptions,
} from '../types'
import { Badge, Banner, Button, Card, Checkbox, Field, Metric, ProgressBar, Spinner } from './ui'

export function MonitorView({
  session,
  onChanged,
}: {
  session: SessionView
  onChanged: () => void
}) {
  const [resp, setResp] = useState<ProgressResponse | null>(null)
  const [pollError, setPollError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState<string | null>(null)

  const poll = useCallback(async () => {
    try {
      const r = await api.progress()
      setResp(r)
      setPollError('')
    } catch (e: any) {
      setPollError(String(e.message || e))
    }
  }, [])

  useEffect(() => {
    poll()
    const id = setInterval(poll, 2000)
    return () => clearInterval(id)
  }, [poll])

  const progress: Progress | undefined = resp?.progress
  const state = (progress?.state || '').toUpperCase()

  // Track how long mongosync has been INITIALIZING so a stuck connection can
  // be flagged — connection failures only appear in the log, not in progress.
  const [initSince, setInitSince] = useState<number | null>(null)
  useEffect(() => {
    setInitSince((prev) =>
      state === 'INITIALIZING' ? (prev ?? Date.now()) : null,
    )
  }, [state])
  const initStuck = initSince !== null && Date.now() - initSince > 30000

  async function runAction(
    name: string,
    fn: () => Promise<unknown>,
  ): Promise<void> {
    setBusy(name)
    setActionError('')
    try {
      await fn()
      await poll()
    } catch (e: any) {
      setActionError(`${name} failed: ${String(e.message || e)}`)
    } finally {
      setBusy(null)
    }
  }

  async function stopSession() {
    setBusy('stop')
    try {
      await api.stopSession()
      onChanged()
    } catch (e: any) {
      setActionError(String(e.message || e))
      setBusy(null)
    }
  }

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          gap: 16,
          flexWrap: 'wrap',
        }}
      >
        <div>
          <h1 className="page-title" style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
            Migration monitor
            {state && (
              <Badge color={stateBadgeColor(state)} dot>
                {state}
              </Badge>
            )}
          </h1>
          <p className="page-subtitle" style={{ marginBottom: 0 }}>
            {session.mode === 'local' ? (
              <>
                Managed local mongosync · API {session.apiBaseUrl}
                {session.pid ? ` · PID ${session.pid}` : ''}
              </>
            ) : (
              <>Attached to remote mongosync · {session.apiBaseUrl}</>
            )}
          </p>
        </div>
        <Button variant="danger" onClick={stopSession} loading={busy === 'stop'}>
          {session.mode === 'local' ? 'Stop & shut down' : 'Detach'}
        </Button>
      </div>

      <div className="stack" style={{ marginTop: 24 }}>
        {session.processExited && (
          <Banner variant="danger">
            <b>mongosync has stopped.</b>{' '}
            {session.exitReason ||
              'The local mongosync process exited unexpectedly.'}{' '}
            Review the logs below, then use “Stop &amp; shut down” to return to
            setup.
          </Banner>
        )}
        {pollError && !session.processExited && (
          <Banner variant="danger">
            Cannot reach mongosync: {pollError}
          </Banner>
        )}
        {actionError && <Banner variant="danger">{actionError}</Banner>}
        {resp && resp.success === false && (resp.error || resp.errorDescription) && (
          <Banner variant="warning">
            mongosync: {resp.error}
            {resp.errorDescription ? ` — ${resp.errorDescription}` : ''}
          </Banner>
        )}

        {state === 'INITIALIZING' && (
          <Banner variant={initStuck ? 'warning' : 'info'}>
            {initStuck
              ? 'mongosync has been initializing for over 30 seconds. It is most likely unable to reach the source or destination cluster — confirm both connection strings point to reachable replica sets, then check the logs below.'
              : 'mongosync is starting up and connecting to the source and destination clusters…'}
          </Banner>
        )}

        {!resp && !pollError && (
          <Card>
            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <Spinner /> Connecting to mongosync…
            </div>
          </Card>
        )}

        <ActionBar
          state={state}
          canCommit={!!progress?.canCommit}
          busy={busy}
          onStart={(opts) => runAction('Start', () => api.start(opts))}
          onPause={() => runAction('Pause', api.pause)}
          onResume={() => runAction('Resume', api.resume)}
          onCommit={() => runAction('Commit', api.commit)}
          onReverse={() => runAction('Reverse', api.reverse)}
        />

        {progress && <ProgressSection progress={progress} />}
        {progress && <DirectionCard progress={progress} />}
        {progress?.verification && <VerificationCard progress={progress} />}
        {progress?.warnings && progress.warnings.length > 0 && (
          <Card title="Warnings">
            <div className="stack">
              {progress.warnings.map((w, i) => (
                <Banner key={i} variant="warning">
                  {w}
                </Banner>
              ))}
            </div>
          </Card>
        )}

        {session.mode === 'local' && (
          <LogsCard
            defaultOpen={state === 'INITIALIZING' || !!session.processExited}
          />
        )}
      </div>
    </div>
  )
}

function ActionBar({
  state,
  canCommit,
  busy,
  onStart,
  onPause,
  onResume,
  onCommit,
  onReverse,
}: {
  state: string
  canCommit: boolean
  busy: string | null
  onStart: (opts: StartOptions) => void
  onPause: () => void
  onResume: () => void
  onCommit: () => void
  onReverse: () => void
}) {
  const [showStart, setShowStart] = useState(false)
  const isIdle = state === 'IDLE' || state === ''

  return (
    <Card title="Controls">
      <div className="btn-row">
        {isIdle && (
          <Button
            variant="primary"
            onClick={() => setShowStart((v) => !v)}
            disabled={busy !== null}
          >
            {showStart ? 'Hide start options' : 'Start migration'}
          </Button>
        )}
        <Button
          onClick={onPause}
          loading={busy === 'Pause'}
          disabled={state !== 'RUNNING' || busy !== null}
        >
          Pause
        </Button>
        <Button
          onClick={onResume}
          loading={busy === 'Resume'}
          disabled={state !== 'PAUSED' || busy !== null}
        >
          Resume
        </Button>
        <Button
          variant="primary"
          onClick={onCommit}
          loading={busy === 'Commit'}
          disabled={!canCommit || busy !== null}
        >
          Commit
        </Button>
        <Button
          onClick={onReverse}
          loading={busy === 'Reverse'}
          disabled={state !== 'COMMITTED' || busy !== null}
        >
          Reverse
        </Button>
      </div>
      {isIdle && !showStart && (
        <p className="card-desc" style={{ marginTop: 12, marginBottom: 0 }}>
          mongosync is idle. Configure start options and begin the migration.
        </p>
      )}
      {!isIdle && state === 'INITIALIZING' && (
        <p className="card-desc" style={{ marginTop: 12, marginBottom: 0 }}>
          mongosync is initializing — controls become available once it has
          connected to both clusters.
        </p>
      )}
      {isIdle && showStart && (
        <StartPanel
          busy={busy === 'Start'}
          onStart={(opts) => {
            onStart(opts)
            setShowStart(false)
          }}
        />
      )}
    </Card>
  )
}

function StartPanel({
  busy,
  onStart,
}: {
  busy: boolean
  onStart: (opts: StartOptions) => void
}) {
  const [reversible, setReversible] = useState(false)
  const [verification, setVerification] = useState(true)
  const [buildIndexes, setBuildIndexes] = useState('afterDataCopy')
  const [namespaces, setNamespaces] = useState<{ db: string; cols: string }[]>([])

  function buildOptions(): StartOptions {
    const includeNamespaces: Namespace[] = namespaces
      .filter((n) => n.db.trim())
      .map((n) => {
        const cols = n.cols
          .split(',')
          .map((c) => c.trim())
          .filter(Boolean)
        return cols.length > 0
          ? { database: n.db.trim(), collections: cols }
          : { database: n.db.trim() }
      })
    const opts: StartOptions = {
      reversible,
      buildIndexes,
      verification: { enabled: verification },
    }
    if (includeNamespaces.length > 0) opts.includeNamespaces = includeNamespaces
    return opts
  }

  return (
    <div className="inline-form">
      <div className="section-label">Start options</div>
      <Checkbox
        checked={reversible}
        onChange={setReversible}
        title="Reversible"
        description="Allow the migration direction to be reversed after commit."
      />
      <Checkbox
        checked={verification}
        onChange={setVerification}
        title="Enable verification"
        description="Run mongosync's embedded data verifier during sync."
      />
      <Field label="Index build strategy">
        <select
          value={buildIndexes}
          onChange={(e) => setBuildIndexes(e.target.value)}
          style={{ maxWidth: 260 }}
        >
          <option value="afterDataCopy">After data copy</option>
          <option value="duringDataCopy">During data copy</option>
          <option value="never">Never</option>
        </select>
      </Field>

      <div className="section-label">Included namespaces (optional)</div>
      <p className="hint" style={{ marginTop: -6, marginBottom: 8 }}>
        Leave empty to migrate everything. Collections are comma-separated;
        leave blank for the whole database.
      </p>
      {namespaces.map((n, i) => (
        <div className="ns-row" key={i}>
          <input
            type="text"
            placeholder="database"
            value={n.db}
            onChange={(e) => {
              const next = [...namespaces]
              next[i] = { ...next[i], db: e.target.value }
              setNamespaces(next)
            }}
          />
          <input
            type="text"
            placeholder="collections (optional)"
            value={n.cols}
            onChange={(e) => {
              const next = [...namespaces]
              next[i] = { ...next[i], cols: e.target.value }
              setNamespaces(next)
            }}
          />
          <Button
            small
            onClick={() => setNamespaces(namespaces.filter((_, j) => j !== i))}
          >
            Remove
          </Button>
        </div>
      ))}
      <Button
        small
        onClick={() => setNamespaces([...namespaces, { db: '', cols: '' }])}
      >
        + Add namespace
      </Button>

      <div style={{ marginTop: 16 }}>
        <Button
          variant="primary"
          onClick={() => onStart(buildOptions())}
          loading={busy}
        >
          Start migration now
        </Button>
      </div>
    </div>
  )
}

function ProgressSection({ progress }: { progress: Progress }) {
  const copied = progress.collectionCopy?.estimatedCopiedBytes
  const total = progress.collectionCopy?.estimatedTotalBytes
  const pct =
    total && total > 0 && copied !== undefined
      ? Math.min(100, (copied / total) * 100)
      : undefined
  const inCopy = (progress.info || '').toLowerCase().includes('collection copy')

  return (
    <Card title="Synchronization progress">
      <div style={{ marginBottom: 6, display: 'flex', justifyContent: 'space-between' }}>
        <span>
          <b>Phase:</b> {progress.info || '—'}
        </span>
        {pct !== undefined && <span className="muted">{pct.toFixed(1)}%</span>}
      </div>
      <ProgressBar
        value={pct}
        indeterminate={
          pct === undefined &&
          (progress.state === 'RUNNING' || progress.state === 'INITIALIZING')
        }
      />
      <div style={{ marginTop: 6 }} className="muted">
        {inCopy ? 'Collection copy: ' : 'Copied: '}
        {formatBytes(copied)} of {formatBytes(total)}
      </div>

      <div className="metrics" style={{ marginTop: 22 }}>
        <Metric
          label="Lag time"
          value={formatDuration(progress.lagTimeSeconds)}
        />
        <Metric
          label="Events applied"
          value={formatNumber(progress.totalEventsApplied)}
        />
        <Metric
          label="Oplog window left"
          value={progress.estimatedOplogTimeRemaining || '—'}
          small
        />
        <Metric
          label="Catch-up estimate"
          value={formatDuration(progress.estimatedSecondsToCEACatchup)}
          small
        />
        <Metric
          label="Writes to destination"
          value={progress.canWrite ? 'allowed' : 'blocked'}
          small
        />
        <Metric
          label="Commit ready"
          value={progress.canCommit ? 'yes' : 'no'}
          small
        />
      </div>
    </Card>
  )
}

function DirectionCard({ progress }: { progress: Progress }) {
  const src = progress.directionMapping?.Source
  const dst = progress.directionMapping?.Destination
  if (!src && !dst) return null
  return (
    <Card title="Direction & connectivity">
      <div className="direction">
        <div className="node">
          <div className="role">Source</div>
          <div className="addr">{src || '—'}</div>
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            ping {formatLatency(progress.source?.pingLatencyMs)}
          </div>
        </div>
        <div className="arrow">→</div>
        <div className="node">
          <div className="role">Destination</div>
          <div className="addr">{dst || '—'}</div>
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            ping {formatLatency(progress.destination?.pingLatencyMs)}
          </div>
        </div>
      </div>
    </Card>
  )
}

function VerificationCard({ progress }: { progress: Progress }) {
  const v = progress.verification
  if (!v) return null
  const phases = [
    { label: 'Source', data: v.source },
    { label: 'Destination', data: v.destination },
  ]
  return (
    <Card
      title="Verification"
      desc="Progress of mongosync's embedded data verifier."
    >
      <div className="row">
        {phases.map(({ label, data }) => (
          <div key={label}>
            <div className="section-label">{label}</div>
            <div className="kv">
              <span className="k">Phase</span>
              <span className="v">{data?.phase || '—'}</span>
            </div>
            <div className="kv">
              <span className="k">Collections scanned</span>
              <span className="v">
                {formatNumber(data?.scannedCollectionCount)} /{' '}
                {formatNumber(data?.totalCollectionCount)}
              </span>
            </div>
            <div className="kv">
              <span className="k">Documents hashed</span>
              <span className="v">
                {formatNumber(data?.hashedDocumentCount)} /{' '}
                {formatNumber(data?.estimatedDocumentCount)}
              </span>
            </div>
            <div className="kv">
              <span className="k">Lag time</span>
              <span className="v">{formatDuration(data?.lagTimeSeconds)}</span>
            </div>
          </div>
        ))}
      </div>
    </Card>
  )
}

function LogsCard({ defaultOpen }: { defaultOpen?: boolean }) {
  const [open, setOpen] = useState(!!defaultOpen)
  const [lines, setLines] = useState<string[]>([])
  const boxRef = useRef<HTMLDivElement>(null)

  // Auto-expand when the caller signals it is worth showing (e.g. a stuck
  // initialization), while still letting the user collapse it afterwards.
  useEffect(() => {
    if (defaultOpen) setOpen(true)
  }, [defaultOpen])

  useEffect(() => {
    if (!open) return
    let active = true
    const load = async () => {
      try {
        const r = await api.logs()
        if (active) setLines(r.lines)
      } catch {
        /* ignore transient log read errors */
      }
    }
    load()
    const id = setInterval(load, 4000)
    return () => {
      active = false
      clearInterval(id)
    }
  }, [open])

  useEffect(() => {
    if (boxRef.current) boxRef.current.scrollTop = boxRef.current.scrollHeight
  }, [lines])

  return (
    <Card>
      <div className="collapsible-head" onClick={() => setOpen((v) => !v)}>
        <span>{open ? '▾' : '▸'}</span> mongosync logs
      </div>
      {open && (
        <div className="logbox" ref={boxRef} style={{ marginTop: 12 }}>
          {lines.length > 0
            ? lines.join('\n')
            : 'No log output yet.'}
        </div>
      )}
    </Card>
  )
}
