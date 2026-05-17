#!/usr/bin/env bash
# Brings up two single-node MongoDB replica sets with keyfile authentication,
# initiates them, creates a user, and seeds sample data into the source —
# a ready-made target for testing mongosync-ui.
set -euo pipefail
cd "$(dirname "$0")"

SOURCE_PORT=27117
DEST_PORT=27118
DB_USER=mongosync
DB_PASS=mongosync

# Sample-data volume — override these to build large collections for testing
# (e.g. to exercise the verifier under memory pressure):
#   SEED_USERS=2000000 SEED_PAD_BYTES=512 ./up.sh   # ~1.2 GB in sample.users
SEED_USERS="${SEED_USERS:-20000}"      # document count in sample.users
SEED_ORDERS="${SEED_ORDERS:-8000}"     # document count in sample.orders
SEED_PAD_BYTES="${SEED_PAD_BYTES:-0}"  # filler bytes added to each user doc

# Refuse to start if the ports are already taken — a collision would silently
# route mongosync to the wrong MongoDB server.
for p in "$SOURCE_PORT" "$DEST_PORT"; do
  if lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "ERROR: port $p is already in use on the host."
    echo "Stop whatever is using it, or edit the ports in docker-compose.yml and up.sh."
    exit 1
  fi
done

# The keyfile is the shared secret for replica-set internal authentication.
if [ ! -f .keyfile ]; then
  echo "==> Generating replica set keyfile"
  openssl rand -base64 756 > .keyfile
  chmod 600 .keyfile
fi

echo "==> Starting MongoDB containers"
docker compose up -d

# msh runs mongosh inside a container, authenticating only when asked. Before
# the first user exists, MongoDB's localhost exception permits the bootstrap
# commands (rs.initiate, createUser) without credentials.
msh() { # <container> <port> <eval> [--auth]
  local container=$1 port=$2 script=$3 auth=${4:-}
  if [ "$auth" = "--auth" ]; then
    docker exec "$container" mongosh --port "$port" --quiet \
      -u "$DB_USER" -p "$DB_PASS" --authenticationDatabase admin \
      --eval "$script"
  else
    docker exec "$container" mongosh --port "$port" --quiet --eval "$script"
  fi
}

wait_ping() { # <container> <port>
  printf '==> Waiting for %s' "$1"
  for _ in $(seq 1 90); do
    if docker exec "$1" mongosh --port "$2" --quiet \
        --eval 'db.runCommand({ping:1}).ok' 2>/dev/null | grep -q 1; then
      echo " ready"; return 0
    fi
    printf '.'; sleep 1
  done
  echo " timed out"; exit 1
}

wait_primary() { # <container> <port>
  printf '==> Waiting for %s primary' "$1"
  for _ in $(seq 1 90); do
    if docker exec "$1" mongosh --port "$2" --quiet \
        --eval 'db.hello().isWritablePrimary' 2>/dev/null | grep -q true; then
      echo " ok"; return 0
    fi
    printf '.'; sleep 1
  done
  echo " timed out"; exit 1
}

wait_ping msui-mongo-source "$SOURCE_PORT"
wait_ping msui-mongo-dest "$DEST_PORT"

echo "==> Initiating replica sets"
msh msui-mongo-source "$SOURCE_PORT" '
  try { rs.initiate({_id:"rs-source",
                     members:[{_id:0, host:"localhost:'"$SOURCE_PORT"'"}]}) }
  catch (e) { if (!/already initialized/i.test(e.message)) throw e }'
msh msui-mongo-dest "$DEST_PORT" '
  try { rs.initiate({_id:"rs-dest",
                     members:[{_id:0, host:"localhost:'"$DEST_PORT"'"}]}) }
  catch (e) { if (!/already initialized/i.test(e.message)) throw e }'

wait_primary msui-mongo-source "$SOURCE_PORT"
wait_primary msui-mongo-dest "$DEST_PORT"

# Create the user via the localhost exception (works only until the first
# user exists). A root user covers every privilege mongosync needs. Reading
# system.users is not permitted under the exception, so just attempt the
# create and tolerate an existing user.
echo "==> Creating database user"
create_user='
  try {
    db.getSiblingDB("admin").createUser({
      user:"'"$DB_USER"'", pwd:"'"$DB_PASS"'",
      roles:[{role:"root", db:"admin"}]});
    print("    user created");
  } catch (e) {
    if (/already exists/i.test(e.message)) print("    user already present");
    else throw e;
  }'
msh msui-mongo-source "$SOURCE_PORT" "$create_user"
msh msui-mongo-dest "$DEST_PORT" "$create_user"

echo "==> Seeding the source cluster (users=${SEED_USERS}, orders=${SEED_ORDERS}, pad=${SEED_PAD_BYTES}B)"
msh msui-mongo-source "$SOURCE_PORT" '
  const userCount  = '"$SEED_USERS"';
  const orderCount = '"$SEED_ORDERS"';
  const padBytes   = '"$SEED_PAD_BYTES"';
  const pad        = padBytes > 0 ? "x".repeat(padBytes) : "";
  const sample = db.getSiblingDB("sample");

  // seed inserts `total` documents into coll in batches, skipping a
  // collection that already holds data.
  function seed(coll, total, makeDoc) {
    if (coll.estimatedDocumentCount() > 0) {
      print("    " + coll.getName() + ": " + coll.estimatedDocumentCount() +
            " docs (already present)");
      return;
    }
    let batch = [];
    for (let i = 0; i < total; i++) {
      batch.push(makeDoc(i));
      if (batch.length === 5000) { coll.insertMany(batch); batch = []; }
      if (i > 0 && i % 100000 === 0) {
        print("      " + coll.getName() + ": " + i + " / " + total);
      }
    }
    if (batch.length) coll.insertMany(batch);
    print("    " + coll.getName() + ": " + coll.estimatedDocumentCount() + " docs");
  }

  seed(sample.users, userCount, i => ({
    _id: i, name: "user" + i, email: "user" + i + "@example.com",
    score: Math.random(), createdAt: new Date(), pad: pad,
  }));
  seed(sample.orders, orderCount, i => ({
    _id: i, userId: userCount > 0 ? i % userCount : 0,
    total: (i * 1.37).toFixed(2), status: ["open", "shipped", "closed"][i % 3],
  }));
' --auth

SRC_URI="mongodb://${DB_USER}:${DB_PASS}@localhost:${SOURCE_PORT}/?replicaSet=rs-source&authSource=admin"
DST_URI="mongodb://${DB_USER}:${DB_PASS}@localhost:${DEST_PORT}/?replicaSet=rs-dest&authSource=admin"

cat <<EOF

==> Test environment ready.

  Source cluster (rs-source)
    ${SRC_URI}

  Destination cluster (rs-dest)
    ${DST_URI}

Paste those into mongosync-ui's "Run locally" setup. The destination starts
empty; the source holds ${SEED_USERS} users + ${SEED_ORDERS} orders in the
"sample" database.

Build bigger collections with:  SEED_USERS=2000000 SEED_PAD_BYTES=512 ./up.sh
Tear down with:                 ./down.sh
EOF
