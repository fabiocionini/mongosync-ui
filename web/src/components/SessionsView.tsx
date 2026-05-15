// SessionsView is the home screen: a browsable list of every migration run.

import {
  elapsedSeconds,
  formatDateTime,
  formatDuration,
  sessionStatusColor,
} from '../format'
import type { ActiveView, SessionRecord } from '../types'
import { Badge, Button, Card } from './ui'

export function SessionsView({
  records,
  active,
  onNew,
  onOpen,
}: {
  records: SessionRecord[]
  active: ActiveView | null
  onNew: () => void
  onOpen: (rec: SessionRecord) => void
}) {
  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'wrap',
          gap: 16,
        }}
      >
        <div>
          <h1 className="page-title">Migration sessions</h1>
          <p className="page-subtitle" style={{ marginBottom: 0 }}>
            Every migration run, with its timeline and outcome.
          </p>
        </div>
        <Button variant="primary" onClick={onNew} disabled={!!active}>
          New migration
        </Button>
      </div>

      {active && (
        <p className="muted" style={{ marginTop: 8, fontSize: 13 }}>
          A migration is currently active — stop it before starting another.
        </p>
      )}

      <div className="stack" style={{ marginTop: 24 }}>
        {records.length === 0 ? (
          <Card>
            <div style={{ textAlign: 'center', padding: '24px 0' }}>
              <p className="muted" style={{ marginTop: 0 }}>
                No migrations yet.
              </p>
              <Button variant="primary" onClick={onNew}>
                Start your first migration
              </Button>
            </div>
          </Card>
        ) : (
          records.map((rec) => (
            <SessionRow key={rec.id} rec={rec} onOpen={() => onOpen(rec)} />
          ))
        )}
      </div>
    </div>
  )
}

function SessionRow({
  rec,
  onOpen,
}: {
  rec: SessionRecord
  onOpen: () => void
}) {
  const isActive = rec.status === 'active'
  const elapsed = elapsedSeconds(rec.startedAt, rec.endedAt)
  const src = rec.source || (rec.mode === 'remote' ? rec.apiBaseUrl : '—')
  const dst = rec.destination || (rec.mode === 'remote' ? '(remote)' : '—')

  return (
    <div className="session-row" onClick={onOpen} role="button" tabIndex={0}>
      <div className="session-row-main">
        <div className="session-row-top">
          <Badge color={sessionStatusColor(rec.status)} dot={isActive}>
            {rec.status}
          </Badge>
          <span className="session-row-mode">{rec.mode}</span>
          {rec.lastState && (
            <span className="muted" style={{ fontSize: 12 }}>
              · {rec.lastState}
            </span>
          )}
        </div>
        <div className="session-row-route mono">
          <span>{src}</span>
          <span className="arrow">→</span>
          <span>{dst}</span>
        </div>
        {rec.status === 'failed' && rec.outcome && (
          <div className="session-row-outcome">{rec.outcome}</div>
        )}
      </div>
      <div className="session-row-times">
        <div>
          <span className="muted">started</span> {formatDateTime(rec.startedAt)}
        </div>
        <div>
          <span className="muted">{isActive ? 'running' : 'ran'}</span>{' '}
          {formatDuration(elapsed)}
        </div>
      </div>
    </div>
  )
}
