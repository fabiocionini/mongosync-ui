# TODO — next steps

Planned work not yet started. Each item lists context, concrete changes, and
risks so it can be picked up later without re-discovery.

## 1. Concurrent migration sessions (Phase 2)

Today the tool runs **one migration at a time**. Phase 1 (session history) was
deliberately built as a *registry* so this can be added without a rewrite —
the data model, per-session directories, storage and the sessions-list UI are
already in place.

**Backend (`internal/`)**
- `session`: remove the single-active guard — the `if s.activeID != ""`
  checks in `StartLocal` / `AttachRemote`. Replace the single `activeID` with
  the set of records whose status is `active`.
- `process`: turn the single `*process.Manager` into a per-session map keyed
  by record id (each local mongosync is an independent process).
- Port allocation: auto-assign a free mongosync API port per local session
  (27182, 27183, …) instead of the user picking one — avoids collisions.
- `reconcileLocked`, `Client`, `InitializingHint`, etc. become per-id rather
  than operating on a single active session.
- `server`: the active-session endpoints (`/api/progress`, `/api/start`,
  `/api/pause|resume|commit|reverse`) need a session id —
  e.g. `/api/sessions/{id}/progress`. `/api/session` (singular) goes away or
  returns the list of active sessions.

**Frontend (`web/src/`)**
- `App.tsx`: allow several active sessions; the monitor is keyed by id.
- Sessions list: "New migration" no longer disabled while one is active;
  show multiple `active` rows.
- A session switcher / per-session monitor view.

**Caveats**
- mongosync writes metadata to the destination cluster — two sessions must
  not target the same destination. Warn (or block) on a shared destination.
- The history cap (`maxRecords = 50`) and registry persistence already
  handle multiplicity.

## 2. Adopt the real LeafyGreen design system

The UI currently *approximates* MongoDB's LeafyGreen look with hand-written
CSS in `web/src/theme.css` and custom components in
`web/src/components/ui.tsx`. This was a deliberate choice for a small, reliable
build — but the real component library gives pixel-exact fidelity.

**Changes**
- Add `@leafygreen-ui/*` packages: `leafygreen-provider`, `button`, `card`,
  `text-input`, `banner`, `badge`, `icon`, `select`, `checkbox`, `toast`,
  `typography`, `palette`, `tokens` (and a progress/loading indicator).
- Wrap the app in `LeafygreenProvider`.
- Replace the custom components in `ui.tsx` (Card, Button, Badge, Banner,
  Field, Checkbox, ProgressBar, Metric, Spinner) with LeafyGreen equivalents.
- Retire most of `theme.css` once components are migrated; keep only layout
  styles (`.session-row`, `.logbox`, grid helpers).

**Risks / watch-outs**
- Heavier bundle and an emotion (CSS-in-JS) runtime — currently the whole UI
  gzips to ~55 kB; LeafyGreen will grow that noticeably.
- LeafyGreen packages have interlocking peer-dependency versions — pin them
  together.
- Re-verify the embedded build (`go:embed all:dist`) still works after the
  dependency change.
