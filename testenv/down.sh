#!/usr/bin/env bash
# Stops the test MongoDB containers and removes their data volumes.
set -euo pipefail
cd "$(dirname "$0")"
docker compose down -v
echo "==> Test environment removed."
