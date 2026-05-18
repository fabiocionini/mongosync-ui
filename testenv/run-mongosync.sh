#!/usr/bin/env bash
# Runs a standalone mongosync against the testenv clusters, so the
# "Attach to remote" flow in mongosync-ui can be tested. mongosync's HTTP
# control API is exposed on http://localhost:27182.
#
# Usage:
#   ./up.sh                # start the test clusters (once)
#   ./run-mongosync.sh     # then run this; attach the UI to :27182
#
# The mongosync binary defaults to the one mongosync-ui downloaded; override
# with MONGOSYNC_BIN=/path/to/mongosync.
set -euo pipefail
cd "$(dirname "$0")"

SOURCE_PORT=27117
DEST_PORT=27118
API_PORT=27182
DB_USER=mongosync
DB_PASS=mongosync

BIN="${MONGOSYNC_BIN:-$HOME/.mongosync-ui/bin/mongosync}"
if [ ! -x "$BIN" ]; then
  echo "ERROR: mongosync binary not found at $BIN"
  echo "Install it once from the mongosync-ui app, or set MONGOSYNC_BIN."
  exit 1
fi

for p in "$SOURCE_PORT" "$DEST_PORT"; do
  if ! lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "ERROR: nothing is listening on port $p — start the clusters first: ./up.sh"
    exit 1
  fi
done
if lsof -nP -iTCP:"$API_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "ERROR: port $API_PORT is already in use (a local mongosync session?)."
  exit 1
fi

LOGDIR="$(pwd)/standalone-logs"
mkdir -p "$LOGDIR"
cd "$LOGDIR"  # mongosync writes a metrics/ directory in its working directory

cat <<EOF
==> Starting standalone mongosync
    API:    http://localhost:${API_PORT}
    logs:   ${LOGDIR}

    In mongosync-ui:  New migration -> Attach to remote
                      URL: http://localhost:${API_PORT}

    Press Ctrl-C to stop.

EOF

exec "$BIN" \
  --cluster0 "mongodb://${DB_USER}:${DB_PASS}@localhost:${SOURCE_PORT}/?replicaSet=rs-source&authSource=admin" \
  --cluster1 "mongodb://${DB_USER}:${DB_PASS}@localhost:${DEST_PORT}/?replicaSet=rs-dest&authSource=admin" \
  --port "${API_PORT}" \
  --logPath "${LOGDIR}"
