# mongosync-ui

A self-contained web UI for configuring, running, monitoring and committing
MongoDB cluster-to-cluster migrations with
[mongosync](https://www.mongodb.com/docs/cluster-to-cluster-sync/).

It downloads and manages the official `mongosync` binary for you, or attaches
to a `mongosync` instance already running on another host — all from a single
executable with a MongoDB-style interface.

## Features

- **Embedded mongosync** — downloads the official binary (verified by SHA-256)
  into the tool's working directory; no separate install needed.
- **Local sessions** — configure source/destination clusters, launch and
  supervise `mongosync`, and shut it down cleanly from the UI.
- **Remote attach** — point the UI at any running `mongosync` HTTP API to
  monitor and control it.
- **Live monitor** — synchronization state, collection-copy progress, lag,
  events applied, index building, verification progress, latency, warnings
  and logs.
- **Lifecycle controls** — start, pause, resume, commit and reverse.
- **Session history** — every run is recorded with its timeline (start /
  elapsed / end), outcome, and a summary of what was migrated (bytes copied,
  documents and collections verified, indexes built). Browse past runs and
  open any one for full details.
- **Single binary** — the React UI is embedded; nothing else to deploy.

## Quick start

Download the binary for your platform and run it:

```bash
./mongosync-ui
```

It serves the UI on <http://localhost:8080> and opens your browser. The home
screen is the **sessions list**. Click **New migration** and choose:

1. **Run locally** — install the mongosync binary, enter your source and
   destination connection strings, and launch mongosync.
2. **Attach to remote** — enter the URL of a running mongosync HTTP API
   (default port `27182`).

Then use the monitor to start the migration, watch progress, and commit. When
a session ends it moves into the history list, where its details remain
available.

> One migration runs at a time; stop the active one before starting another.

### Options

| Flag         | Default            | Description                                  |
|--------------|--------------------|----------------------------------------------|
| `--port`     | `8080`             | Port for the mongosync-ui web interface      |
| `--workdir`  | `~/.mongosync-ui`  | Directory for the mongosync binary and data  |
| `--open`     | `true`             | Open the UI in a browser on startup          |
| `--version`  |                    | Print version and exit                       |

### Working directory layout

```
~/.mongosync-ui/
├── bin/mongosync            downloaded mongosync binary
├── sessions.json            session registry (history, capped at 50 runs)
└── sessions/<id>/           one directory per migration run
    ├── mongosync.yaml       generated mongosync config (mode 0600)
    ├── mongosync.log        mongosync's structured log
    ├── process.log          captured process stdout/stderr
    └── metrics/             mongosync metrics output
```

## Trying it out

The `testenv/` directory spins up two single-node MongoDB replica sets in
Docker (with keyfile authentication, since mongosync requires it) as a
ready-made migration target:

```bash
cd testenv
./up.sh      # start the clusters, seed sample data, print connection strings
./down.sh    # tear everything down
```

Recreate the environment (`down.sh && up.sh`) before each test run —
mongosync cannot start a fresh sync onto clusters that already hold a
finished one.

## Building from source

Requires **Go 1.26+** and **Node.js 20+**.

```bash
# Build a binary for the current platform
make build && ./dist/mongosync-ui

# Cross-compile release binaries for macOS, Linux and Windows
make release        # output in ./dist/
```

### Development

Run the Go backend and the Vite dev server (with hot reload) separately:

```bash
# Terminal 1 — backend on :8080
make dev

# Terminal 2 — UI on :5173, proxying /api to :8080
cd web && npm install && npm run dev
```

## Architecture

```
cmd/mongosync-ui      entrypoint and flags
internal/binary       downloads/verifies/extracts the mongosync binary
internal/process      supervises the local mongosync child process
internal/client       wraps the mongosync HTTP control API
internal/session      session registry: records, history and per-run state
internal/server       REST API + serves the embedded SPA
web/                  React + TypeScript UI (Vite), embedded via go:embed
```

The Go server exposes a REST API (`/api/...`) that the SPA consumes:
`/api/sessions` lists the history, `/api/session` is the active session, and
for an active session the server proxies the mongosync HTTP API at
`/api/v1/{progress,start,pause,resume,commit,reverse}`.

The single-active-session model is built as a registry, so running multiple
concurrent migrations can be added later without reworking it.

## Security notes

- The mongosync HTTP API has **no authentication** and binds to localhost by
  default. When attaching to a remote instance, ensure it is reachable only
  over a trusted network.
- Cluster connection strings (with credentials) are written to each session's
  `mongosync.yaml` with `0600` permissions and are sent only to mongosync.
- Connection strings stored in the session history have their passwords
  redacted.
