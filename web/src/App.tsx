// App is the root component. It tracks the session registry and the active
// session, and routes between the sessions list, setup wizard, live monitor
// and historical session details.

import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api'
import { MonitorView } from './components/MonitorView'
import { SetupView } from './components/SetupView'
import { SessionsView } from './components/SessionsView'
import { SessionDetailsView } from './components/SessionDetailsView'
import { Badge, Banner, Spinner } from './components/ui'
import { LeafLogo } from './components/icons'
import type { SessionRecord, SessionResponse } from './types'

type View =
  | { name: 'sessions' }
  | { name: 'setup' }
  | { name: 'monitor' }
  | { name: 'details'; id: string }

export default function App() {
  const [session, setSession] = useState<SessionResponse | null>(null)
  const [records, setRecords] = useState<SessionRecord[]>([])
  const [view, setView] = useState<View>({ name: 'sessions' })
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const monitoredId = useRef<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [s, list] = await Promise.all([api.getSession(), api.sessions()])
      setSession(s)
      setRecords(list.records)
      setError('')
    } catch (e: any) {
      setError(String(e.message || e))
    } finally {
      setLoaded(true)
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, 2000)
    return () => clearInterval(id)
  }, [refresh])

  const active = session?.active ?? null

  // When the monitored session ends, slide into its details view.
  useEffect(() => {
    if (view.name !== 'monitor') return
    if (active) {
      monitoredId.current = active.record.id
    } else if (loaded && session) {
      setView(
        monitoredId.current
          ? { name: 'details', id: monitoredId.current }
          : { name: 'sessions' },
      )
    }
  }, [view, active, loaded, session])

  // A monitor view whose session has ended resolves to that session's details
  // (or the list) — computed during render so the UI never shows a blank
  // frame, regardless of when the routing effect above runs.
  const resolved: View =
    view.name === 'monitor' && !active
      ? monitoredId.current
        ? { name: 'details', id: monitoredId.current }
        : { name: 'sessions' }
      : view

  function openRecord(rec: SessionRecord) {
    if (rec.status === 'active') {
      monitoredId.current = rec.id
      setView({ name: 'monitor' })
    } else {
      setView({ name: 'details', id: rec.id })
    }
  }

  return (
    <div className="app">
      <header className="topbar">
        <div
          className="brand"
          onClick={() => setView({ name: 'sessions' })}
          style={{ cursor: 'pointer' }}
        >
          <LeafLogo />
          <span>
            mongosync <span className="sub">UI</span>
          </span>
          {session?.version && (
            <span className="brand-version">{session.version}</span>
          )}
        </div>
        <div className="spacer" />
        {session &&
          (active ? (
            <Badge
              color={active.record.mode === 'local' ? 'green' : 'blue'}
              dot
            >
              {active.record.mode === 'local' ? 'Local' : 'Remote'} session
              active
            </Badge>
          ) : (
            <Badge color="gray">No active session</Badge>
          ))}
      </header>

      <main className="main">
        {!loaded && (
          <div className="center-screen">
            <Spinner /> Loading…
          </div>
        )}

        {loaded && error && !session && (
          <Banner variant="danger">
            Could not contact the mongosync-ui server: {error}
          </Banner>
        )}

        {loaded && session && resolved.name === 'sessions' && (
          <SessionsView
            records={records}
            active={active}
            onNew={() => setView({ name: 'setup' })}
            onOpen={openRecord}
            onChanged={refresh}
          />
        )}

        {loaded && session && resolved.name === 'setup' && (
          <SetupView
            binary={session.binary}
            hasActive={!!active}
            onChanged={refresh}
            onStarted={async () => {
              await refresh()
              setView({ name: 'monitor' })
            }}
            onBack={() => setView({ name: 'sessions' })}
          />
        )}

        {loaded && session && resolved.name === 'monitor' && active && (
          <MonitorView
            active={active}
            onChanged={refresh}
            onBack={() => setView({ name: 'sessions' })}
          />
        )}

        {loaded && resolved.name === 'details' && (
          <SessionDetailsView
            id={resolved.id}
            onBack={() => setView({ name: 'sessions' })}
          />
        )}
      </main>
    </div>
  )
}
