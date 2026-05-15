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
  events applied, verification progress, latency, warnings and logs.
- **Lifecycle controls** — start, pause, resume, commit and reverse.
- **Single binary** — the React UI is embedded; nothing else to deploy.

## Quick start

Download the binary for your platform and run it:

```bash
./mongosync-ui
```

It serves the UI on <http://localhost:8080> and opens your browser. From there:

1. **Run locally** — install the mongosync binary, enter your source and
   destination connection strings, and launch mongosync.
2. **Attach to remote** — enter the URL of a running mongosync HTTP API
   (default port `27182`).

Then use the monitor to start the migration, watch progress, and commit.

### Options

| Flag         | Default            | Description                                   |
|--------------|--------------------|-----------------------------------------------|
| `--port`     | `8080`             | Port for the mongosync-ui web interface       |
| `--workdir`  | `~/.mongosync-ui`  | Directory for the binary, config, logs, state |
| `--open`     | `true`             | Open the UI in a browser on startup           |
| `--version`  |                    | Print version and exit                        |

### Working directory layout

```
~/.mongosync-ui/
├── bin/mongosync          downloaded mongosync binary
├── config/mongosync.yaml  generated mongosync configuration (mode 0600)
├── logs/                  mongosync + process logs
└── state.json             persisted session state
```

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
internal/session      single-session state (local | remote) and config
internal/server       REST API + serves the embedded SPA
web/                  React + TypeScript UI (Vite), embedded via go:embed
```

The Go server exposes a small REST API (`/api/...`) that the SPA consumes;
for an active session it proxies to the mongosync HTTP API at
`/api/v1/{progress,start,pause,resume,commit,reverse}`.

## Security notes

- The mongosync HTTP API has **no authentication** and binds to localhost by
  default. When attaching to a remote instance, ensure it is reachable only
  over a trusted network.
- Cluster connection strings (with credentials) are written to
  `config/mongosync.yaml` with `0600` permissions and are sent only to
  mongosync.
