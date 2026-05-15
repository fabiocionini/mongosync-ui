// SetupView is the first screen: choose between launching a local mongosync
// or attaching to an already-running remote instance.

import { useEffect, useState } from 'react'
import { api } from '../api'
import type { SessionView } from '../types'
import { Badge, Banner, Button, Card, Field, ProgressBar } from './ui'

export function SetupView({
  session,
  onChanged,
}: {
  session: SessionView
  onChanged: () => void
}) {
  const [tab, setTab] = useState<'local' | 'remote'>('local')

  return (
    <div>
      <h1 className="page-title">Set up a migration</h1>
      <p className="page-subtitle">
        Run a managed mongosync on this machine, or connect to one already
        running elsewhere.
      </p>

      <div className="tabs">
        <button
          className={`tab ${tab === 'local' ? 'active' : ''}`}
          onClick={() => setTab('local')}
        >
          Run locally
        </button>
        <button
          className={`tab ${tab === 'remote' ? 'active' : ''}`}
          onClick={() => setTab('remote')}
        >
          Attach to remote
        </button>
      </div>

      {tab === 'local' ? (
        <LocalSetup session={session} onChanged={onChanged} />
      ) : (
        <RemoteSetup onChanged={onChanged} />
      )}
    </div>
  )
}

function BinarySection({
  session,
  onChanged,
}: {
  session: SessionView
  onChanged: () => void
}) {
  const bin = session.binary
  const [versions, setVersions] = useState<string[]>([])
  const [selected, setSelected] = useState('')
  const [loadingVersions, setLoadingVersions] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    if (bin.state === 'installed') return
    setLoadingVersions(true)
    api
      .binaryVersions()
      .then((r) => {
        setVersions(r.versions)
        setSelected(r.versions[0] ?? '')
      })
      .catch((e) => setErr(String(e.message || e)))
      .finally(() => setLoadingVersions(false))
  }, [bin.state])

  async function install() {
    setErr('')
    try {
      await api.installBinary(selected)
      onChanged()
    } catch (e: any) {
      setErr(String(e.message || e))
    }
  }

  if (bin.state === 'installed') {
    return (
      <Banner variant="success">
        mongosync <b>{bin.version}</b> is installed and ready.
      </Banner>
    )
  }

  if (bin.state === 'downloading' || bin.state === 'extracting') {
    return (
      <div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
          <span>
            {bin.state === 'downloading' ? 'Downloading' : 'Extracting'} mongosync{' '}
            {bin.version}…
          </span>
          <span className="muted">
            {bin.state === 'downloading' ? `${bin.progress}%` : ''}
          </span>
        </div>
        <ProgressBar
          value={bin.progress}
          indeterminate={bin.state === 'extracting'}
        />
      </div>
    )
  }

  return (
    <div>
      <p className="card-desc">
        The official mongosync binary is downloaded from MongoDB and stored
        inside this tool's working directory.
      </p>
      {bin.state === 'error' && bin.error && (
        <div style={{ marginBottom: 12 }}>
          <Banner variant="danger">Install failed: {bin.error}</Banner>
        </div>
      )}
      {err && (
        <div style={{ marginBottom: 12 }}>
          <Banner variant="danger">{err}</Banner>
        </div>
      )}
      <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end' }}>
        <div className="field" style={{ flex: 1, marginBottom: 0 }}>
          <label>mongosync version</label>
          <select
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
            disabled={loadingVersions || versions.length === 0}
          >
            {loadingVersions && <option>Loading versions…</option>}
            {versions.map((v, i) => (
              <option key={v} value={v}>
                {v}
                {i === 0 ? ' (latest)' : ''}
              </option>
            ))}
          </select>
        </div>
        <Button
          variant="primary"
          onClick={install}
          disabled={!selected || loadingVersions}
        >
          Download &amp; install
        </Button>
      </div>
    </div>
  )
}

function LocalSetup({
  session,
  onChanged,
}: {
  session: SessionView
  onChanged: () => void
}) {
  const [sourceUri, setSourceUri] = useState('')
  const [destinationUri, setDestinationUri] = useState('')
  const [port, setPort] = useState(27182)
  const [submitting, setSubmitting] = useState(false)
  const [err, setErr] = useState('')

  const binaryReady = session.binary.state === 'installed'

  async function start() {
    setErr('')
    setSubmitting(true)
    try {
      await api.startLocal({
        sourceUri: sourceUri.trim(),
        destinationUri: destinationUri.trim(),
        port,
        version: session.binary.version ?? '',
      })
      onChanged()
    } catch (e: any) {
      setErr(String(e.message || e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="stack">
      <Card title="mongosync binary">
        <BinarySection session={session} onChanged={onChanged} />
      </Card>

      <Card
        title="Cluster connections"
        desc="Connection strings are written to a local config file readable only by you. They are never sent anywhere except to mongosync."
      >
        <Field
          label="Source cluster (cluster0)"
          hint="The cluster mongosync reads from."
        >
          <input
            type="text"
            placeholder="mongodb+srv://user:pass@source.example.mongodb.net"
            value={sourceUri}
            onChange={(e) => setSourceUri(e.target.value)}
          />
        </Field>
        <Field
          label="Destination cluster (cluster1)"
          hint="The cluster mongosync writes to."
        >
          <input
            type="text"
            placeholder="mongodb+srv://user:pass@target.example.mongodb.net"
            value={destinationUri}
            onChange={(e) => setDestinationUri(e.target.value)}
          />
        </Field>
        <Field
          label="mongosync API port"
          hint="Local port for the mongosync control API."
        >
          <input
            type="number"
            value={port}
            onChange={(e) => setPort(Number(e.target.value) || 27182)}
            style={{ maxWidth: 160 }}
          />
        </Field>

        {err && (
          <div style={{ marginBottom: 12 }}>
            <Banner variant="danger">{err}</Banner>
          </div>
        )}
        {!binaryReady && (
          <div style={{ marginBottom: 12 }}>
            <Banner variant="info">
              Install the mongosync binary above before launching a migration.
            </Banner>
          </div>
        )}

        <Button
          variant="primary"
          onClick={start}
          loading={submitting}
          disabled={!binaryReady || !sourceUri.trim() || !destinationUri.trim()}
        >
          Launch mongosync
        </Button>
      </Card>
    </div>
  )
}

function RemoteSetup({ onChanged }: { onChanged: () => void }) {
  const [url, setUrl] = useState('http://localhost:27182')
  const [submitting, setSubmitting] = useState(false)
  const [err, setErr] = useState('')

  async function attach() {
    setErr('')
    setSubmitting(true)
    try {
      await api.attachRemote(url.trim())
      onChanged()
    } catch (e: any) {
      setErr(String(e.message || e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card
      title="Attach to a running mongosync"
      desc="Monitor and control a mongosync instance that is already running on another host or container."
    >
      <div style={{ marginBottom: 12 }}>
        <Badge color="blue">read &amp; control</Badge>
      </div>
      <Field
        label="mongosync API URL"
        hint="The address of the mongosync HTTP control API (default port 27182)."
      >
        <input
          type="text"
          placeholder="http://10.0.0.5:27182"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
      </Field>
      {err && (
        <div style={{ marginBottom: 12 }}>
          <Banner variant="danger">{err}</Banner>
        </div>
      )}
      <Banner variant="info">
        The mongosync API has no authentication, so it must be reachable from
        this machine over a trusted network.
      </Banner>
      <div style={{ marginTop: 16 }}>
        <Button
          variant="primary"
          onClick={attach}
          loading={submitting}
          disabled={!url.trim()}
        >
          Attach
        </Button>
      </div>
    </Card>
  )
}
