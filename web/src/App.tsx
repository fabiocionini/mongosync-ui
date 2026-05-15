// App is the root component: it tracks the session state and routes between
// the setup wizard and the live migration monitor.

import { useCallback, useEffect, useState } from 'react'
import { api } from './api'
import { MonitorView } from './components/MonitorView'
import { SetupView } from './components/SetupView'
import { Badge, Banner, Spinner } from './components/ui'
import { LeafLogo } from './components/icons'
import type { SessionView } from './types'

export default function App() {
  const [session, setSession] = useState<SessionView | null>(null)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const s = await api.getSession()
      setSession(s)
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

  const active = session && session.mode !== 'none'

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <LeafLogo />
          <span>
            mongosync <span className="sub">UI</span>
          </span>
        </div>
        <div className="spacer" />
        {session && (
          <div className="topinfo">
            {active ? (
              <Badge color={session.mode === 'local' ? 'green' : 'blue'} dot>
                {session.mode === 'local' ? 'Local session' : 'Remote session'}
              </Badge>
            ) : (
              <Badge color="gray">No active session</Badge>
            )}
          </div>
        )}
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

        {loaded && session && !active && (
          <SetupView session={session} onChanged={refresh} />
        )}

        {loaded && session && active && (
          <MonitorView session={session} onChanged={refresh} />
        )}
      </main>
    </div>
  )
}
