// SessionsView is the home screen: a browsable list of every migration run.

import { useState } from 'react'
import { api } from '../api'
import {
  elapsedSeconds,
  formatDateTime,
  formatDuration,
  sessionStatusColor,
} from '../format'
import type { ActiveView, SessionRecord } from '../types'
import { Badge, Button, Card, ConfirmDialog } from './ui'

export function SessionsView({
  records,
  active,
  onNew,
  onOpen,
  onChanged,
}: {
  records: SessionRecord[]
  active: ActiveView | null
  onNew: () => void
  onOpen: (rec: SessionRecord) => void
  onChanged: () => void
}) {
  const [toDelete, setToDelete] = useState<SessionRecord | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')

  async function confirmDelete() {
    if (!toDelete) return
    setDeleting(true)
    setDeleteError('')
    try {
      await api.deleteSession(toDelete.id)
      setToDelete(null)
      onChanged()
    } catch (e: any) {
      setDeleteError(String(e.message || e))
    } finally {
      setDeleting(false)
    }
  }

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
            <SessionRow
              key={rec.id}
              rec={rec}
              onOpen={() => onOpen(rec)}
              onDelete={() => {
                setDeleteError('')
                setToDelete(rec)
              }}
            />
          ))
        )}
      </div>

      {toDelete && (
        <ConfirmDialog
          title="Delete session"
          danger
          confirmLabel="Delete"
          busy={deleting}
          error={deleteError}
          message="Permanently delete this migration session? Its history record, logs and configuration files are removed. This cannot be undone."
          onConfirm={confirmDelete}
          onCancel={() => {
            setToDelete(null)
            setDeleteError('')
          }}
        />
      )}
    </div>
  )
}

function SessionRow({
  rec,
  onOpen,
  onDelete,
}: {
  rec: SessionRecord
  onOpen: () => void
  onDelete: () => void
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
      <div className="session-row-side">
        <div className="session-row-times">
          <div>
            <span className="muted">started</span>{' '}
            {formatDateTime(rec.startedAt)}
          </div>
          <div>
            <span className="muted">{isActive ? 'running' : 'ran'}</span>{' '}
            {formatDuration(elapsed)}
          </div>
        </div>
        {!isActive && (
          <Button
            small
            variant="danger"
            onClick={(e) => {
              e.stopPropagation()
              onDelete()
            }}
          >
            Delete
          </Button>
        )}
      </div>
    </div>
  )
}
