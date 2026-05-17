// Package session manages migration sessions. Each run — local or attached to
// a remote mongosync — is recorded as a Record in a persistent registry, so
// the history of every migration is browsable. At most one session is active
// at a time; the registry model is ready to lift that limit later.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fabiocionini/mongosync-ui/internal/binary"
	"github.com/fabiocionini/mongosync-ui/internal/client"
	"github.com/fabiocionini/mongosync-ui/internal/process"
)

// Session modes.
const (
	ModeLocal  = "local"
	ModeRemote = "remote"
)

// Session statuses.
const (
	StatusActive    = "active"
	StatusCommitted = "committed"
	StatusStopped   = "stopped"
	StatusFailed    = "failed"
)

// Namespace selects a database and, optionally, specific collections.
type Namespace struct {
	Database    string   `json:"database"`
	Collections []string `json:"collections,omitempty"`
}

// MigrationConfig holds everything needed to launch a local mongosync.
type MigrationConfig struct {
	SourceURI      string `json:"sourceUri"`
	DestinationURI string `json:"destinationUri"`
	Port           int    `json:"port"`
	Version        string `json:"version"`
	// EnableVerifierPersistence passes mongosync's undocumented
	// --enableVerifierPersistence flag, which keeps the data verifier's state
	// out of RAM — useful for very large collections.
	EnableVerifierPersistence bool `json:"enableVerifierPersistence"`
}

// Record is one migration session — current or historical.
type Record struct {
	ID               string     `json:"id"`
	Mode             string     `json:"mode"`
	APIBaseURL       string     `json:"apiBaseUrl"`
	Port             int        `json:"port,omitempty"`
	Source           string     `json:"source"`      // display string, credentials redacted
	Destination      string     `json:"destination"` // display string, credentials redacted
	MongosyncVersion string     `json:"mongosyncVersion,omitempty"`
	StartedAt        time.Time  `json:"startedAt"`
	EndedAt          *time.Time `json:"endedAt,omitempty"`
	Status           string     `json:"status"`
	LastState        string     `json:"lastState,omitempty"` // last observed mongosync state
	Outcome          string     `json:"outcome,omitempty"`   // failure reason, if any
	Summary          *Summary   `json:"summary,omitempty"`   // peak observed progress
}

// Summary is a snapshot of a migration's measurable progress. Each field holds
// the largest value observed, so mongosync resetting its counters after commit
// cannot erase the real totals.
type Summary struct {
	Phase               string `json:"phase,omitempty"`
	CopiedBytes         int64  `json:"copiedBytes,omitempty"`
	TotalBytes          int64  `json:"totalBytes,omitempty"`
	EventsApplied       int64  `json:"eventsApplied,omitempty"`
	IndexesBuilt        int    `json:"indexesBuilt,omitempty"`
	TotalIndexes        int    `json:"totalIndexes,omitempty"`
	VerifiedDocuments   int64  `json:"verifiedDocuments,omitempty"`
	EstimatedDocuments  int64  `json:"estimatedDocuments,omitempty"`
	VerifiedCollections int    `json:"verifiedCollections,omitempty"`
	TotalCollections    int    `json:"totalCollections,omitempty"`
}

// store is the on-disk shape of the registry.
type store struct {
	Records []*Record `json:"records"`
}

// maxRecords caps the registry so it cannot grow without bound.
const maxRecords = 50

// Session owns the migration registry and the (single) active process.
type Session struct {
	workdir string
	bin     *binary.Manager
	proc    *process.Manager

	mu       sync.Mutex
	records  []*Record // oldest first
	activeID string
}

// New builds a Session, loading any previous registry from disk.
func New(workdir string, bin *binary.Manager) *Session {
	s := &Session{workdir: workdir, bin: bin, proc: process.New()}
	s.load()
	return s
}

func (s *Session) sessionsRoot() string { return filepath.Join(s.workdir, "sessions") }
func (s *Session) storePath() string    { return filepath.Join(s.workdir, "sessions.json") }
func (s *Session) sessionDir(id string) string {
	return filepath.Join(s.sessionsRoot(), id)
}
func (s *Session) configPath(id string) string {
	return filepath.Join(s.sessionDir(id), "mongosync.yaml")
}
func (s *Session) procLogPath(id string) string {
	return filepath.Join(s.sessionDir(id), "process.log")
}

// MongosyncLogPath is the structured log file mongosync writes for a session.
func (s *Session) MongosyncLogPath(id string) string {
	return filepath.Join(s.sessionDir(id), "mongosync.log")
}

// ProcLogPath is the captured stdout/stderr of a session's mongosync process.
func (s *Session) ProcLogPath(id string) string { return s.procLogPath(id) }

// load reads the registry and reconciles any session left active by a prior
// run: still reachable ones are kept (as unmanaged remote sessions), the rest
// are finalized.
func (s *Session) load() {
	raw, err := os.ReadFile(s.storePath())
	if err == nil {
		var st store
		if json.Unmarshal(raw, &st) == nil {
			s.records = st.Records
		}
	}
	for _, r := range s.records {
		if r.Status != StatusActive {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := client.New(r.APIBaseURL).Ping(ctx)
		cancel()
		if err == nil {
			// The process is no longer ours to supervise; keep monitoring it
			// as an attached remote session.
			r.Mode = ModeRemote
			s.activeID = r.ID
		} else {
			finalize(r, deriveStatus(r, nil))
		}
	}
	s.persistLocked()
}

func (s *Session) persistLocked() {
	if len(s.records) > maxRecords {
		s.records = s.records[len(s.records)-maxRecords:]
	}
	data, err := json.MarshalIndent(store{Records: s.records}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.storePath(), data, 0o600)
}

func (s *Session) recordLocked(id string) *Record {
	for _, r := range s.records {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// reconcileLocked finalizes the active session if its local process has died.
func (s *Session) reconcileLocked() {
	if s.activeID == "" {
		return
	}
	r := s.recordLocked(s.activeID)
	if r == nil || r.Status != StatusActive {
		s.activeID = ""
		return
	}
	if r.Mode == ModeLocal && !s.proc.Running() {
		if r.Outcome == "" {
			r.Outcome = lastMongosyncError(s.MongosyncLogPath(r.ID))
		}
		finalize(r, deriveStatus(r, s.proc.ExitError()))
		s.activeID = ""
		s.persistLocked()
	}
}

// finalize stamps a record as ended with the given status.
func finalize(r *Record, status string) {
	if r.EndedAt == nil {
		now := time.Now().UTC()
		r.EndedAt = &now
	}
	r.Status = status
}

// deriveStatus chooses an end status from the last observed state.
func deriveStatus(r *Record, exitErr error) string {
	if r.LastState == "COMMITTED" {
		return StatusCommitted
	}
	if exitErr != nil {
		return StatusFailed
	}
	return StatusStopped
}

// Records returns every session, newest first.
func (s *Session) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()
	out := make([]Record, 0, len(s.records))
	for i := len(s.records) - 1; i >= 0; i-- {
		out = append(out, *s.records[i])
	}
	return out
}

// Record returns a single session by id.
func (s *Session) Record(id string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()
	if r := s.recordLocked(id); r != nil {
		return *r, true
	}
	return Record{}, false
}

// Active returns the currently active session, if any.
func (s *Session) Active() (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()
	if r := s.recordLocked(s.activeID); r != nil {
		return *r, true
	}
	return Record{}, false
}

// Running reports whether a session is currently active.
func (s *Session) Running() bool {
	_, ok := s.Active()
	return ok
}

// Client returns a control client for the active session, or nil when idle.
func (s *Session) Client() *client.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()
	if r := s.recordLocked(s.activeID); r != nil {
		return client.New(r.APIBaseURL)
	}
	return nil
}

// StartLocal writes a mongosync config, launches the binary, waits for its API
// and records the session.
func (s *Session) StartLocal(cfg MigrationConfig) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()

	if s.activeID != "" {
		return Record{}, fmt.Errorf("a migration is already active; stop it first")
	}
	if !s.bin.Installed() {
		return Record{}, fmt.Errorf("the mongosync binary is not installed yet")
	}
	if strings.TrimSpace(cfg.SourceURI) == "" || strings.TrimSpace(cfg.DestinationURI) == "" {
		return Record{}, fmt.Errorf("source and destination connection strings are required")
	}
	if cfg.Port == 0 {
		cfg.Port = 27182
	}

	id := newID()
	dir := s.sessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Record{}, err
	}
	if err := writeConfigYAML(s.configPath(id), cfg, dir); err != nil {
		return Record{}, fmt.Errorf("write mongosync config: %w", err)
	}

	rec := &Record{
		ID:               id,
		Mode:             ModeLocal,
		APIBaseURL:       fmt.Sprintf("http://localhost:%d", cfg.Port),
		Port:             cfg.Port,
		Source:           redactURI(cfg.SourceURI),
		Destination:      redactURI(cfg.DestinationURI),
		MongosyncVersion: cfg.Version,
		StartedAt:        time.Now().UTC(),
		Status:           StatusActive,
	}
	s.records = append(s.records, rec)
	s.activeID = id
	s.persistLocked()

	var extraArgs []string
	if cfg.EnableVerifierPersistence {
		extraArgs = append(extraArgs, "--enableVerifierPersistence")
	}

	if err := s.proc.Start(s.bin.BinaryPath(), s.configPath(id), s.procLogPath(id), dir, extraArgs); err != nil {
		rec.Outcome = "failed to launch mongosync: " + err.Error()
		finalize(rec, StatusFailed)
		s.activeID = ""
		s.persistLocked()
		return Record{}, fmt.Errorf("launch mongosync: %w", err)
	}

	if err := client.New(rec.APIBaseURL).WaitReady(60 * time.Second); err != nil {
		_ = s.proc.Stop()
		rec.Outcome = lastMongosyncError(s.MongosyncLogPath(id))
		if rec.Outcome == "" {
			rec.Outcome = err.Error()
		}
		finalize(rec, StatusFailed)
		s.activeID = ""
		s.persistLocked()
		return Record{}, err
	}

	s.persistLocked()
	return *rec, nil
}

// AttachRemote connects the UI to an already-running mongosync and records it.
func (s *Session) AttachRemote(apiURL string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked()

	if s.activeID != "" {
		return Record{}, fmt.Errorf("a migration is already active; stop it first")
	}
	apiURL = normalizeURL(apiURL)
	if apiURL == "" {
		return Record{}, fmt.Errorf("a mongosync API URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.New(apiURL).Ping(ctx); err != nil {
		return Record{}, fmt.Errorf("cannot reach mongosync at %s: %w", apiURL, err)
	}

	id := newID()
	if err := os.MkdirAll(s.sessionDir(id), 0o755); err != nil {
		return Record{}, err
	}
	rec := &Record{
		ID:         id,
		Mode:       ModeRemote,
		APIBaseURL: apiURL,
		StartedAt:  time.Now().UTC(),
		Status:     StatusActive,
	}
	s.records = append(s.records, rec)
	s.activeID = id
	s.persistLocked()
	return *rec, nil
}

// Stop ends the active session, terminating the local process if applicable.
func (s *Session) Stop() error {
	s.mu.Lock()
	r := s.recordLocked(s.activeID)
	isLocal := r != nil && r.Mode == ModeLocal
	s.mu.Unlock()

	if isLocal {
		_ = s.proc.Stop()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.recordLocked(s.activeID); r != nil {
		finalize(r, deriveStatus(r, nil))
		s.activeID = ""
		s.persistLocked()
	}
	return nil
}

// UpdateProgress records the latest observed mongosync state, the progress
// summary, and (for remote sessions) the source/destination discovered from
// the progress response.
func (s *Session) UpdateProgress(state, source, destination string, snap Summary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recordLocked(s.activeID)
	if r == nil {
		return
	}
	changed := false
	if state != "" && state != r.LastState {
		r.LastState = state
		changed = true
	}
	if r.Source == "" && source != "" {
		r.Source = source
		changed = true
	}
	if r.Destination == "" && destination != "" {
		r.Destination = destination
		changed = true
	}
	if r.Summary == nil {
		r.Summary = &Summary{}
	}
	if mergeSummary(r.Summary, snap) {
		changed = true
	}
	if changed {
		s.persistLocked()
	}
}

// mergeSummary folds a fresh snapshot into dst, keeping the largest value of
// each metric. It reports whether anything changed.
func mergeSummary(dst *Summary, snap Summary) bool {
	changed := false
	if snap.Phase != "" && snap.Phase != dst.Phase {
		dst.Phase = snap.Phase
		changed = true
	}
	maxI64 := func(d *int64, v int64) {
		if v > *d {
			*d, changed = v, true
		}
	}
	maxInt := func(d *int, v int) {
		if v > *d {
			*d, changed = v, true
		}
	}
	maxI64(&dst.CopiedBytes, snap.CopiedBytes)
	maxI64(&dst.TotalBytes, snap.TotalBytes)
	maxI64(&dst.EventsApplied, snap.EventsApplied)
	maxInt(&dst.IndexesBuilt, snap.IndexesBuilt)
	maxInt(&dst.TotalIndexes, snap.TotalIndexes)
	maxI64(&dst.VerifiedDocuments, snap.VerifiedDocuments)
	maxI64(&dst.EstimatedDocuments, snap.EstimatedDocuments)
	maxInt(&dst.VerifiedCollections, snap.VerifiedCollections)
	maxInt(&dst.TotalCollections, snap.TotalCollections)
	return changed
}

// InitializingHint explains the active session's INITIALIZING state from its
// log. It returns a message and whether the situation indicates a problem.
func (s *Session) InitializingHint() (string, bool) {
	s.mu.Lock()
	id := s.activeID
	s.mu.Unlock()
	if id == "" {
		return "", false
	}
	return initHintFromLog(s.MongosyncLogPath(id))
}

// newID returns a unique, sortable session id.
func newID() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// normalizeURL ensures the URL has a scheme and host.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}

// redactURI masks the password in a MongoDB connection string for display,
// leaving the rest of the string byte-for-byte intact.
func redactURI(raw string) string {
	raw = strings.TrimSpace(raw)
	scheme := strings.Index(raw, "://")
	if scheme < 0 {
		return raw
	}
	rest := raw[scheme+3:]
	at := strings.Index(rest, "@")
	if at < 0 {
		return raw // no userinfo
	}
	userinfo := rest[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 {
		return raw // username only, no password
	}
	return raw[:scheme+3] + userinfo[:colon] + ":***" + rest[at:]
}

// writeConfigYAML emits a minimal mongosync configuration file. It holds
// connection strings with credentials, so it is written 0600.
func writeConfigYAML(path string, cfg MigrationConfig, logDir string) error {
	var b strings.Builder
	b.WriteString("# Generated by mongosync-ui — do not edit while a session is running.\n")
	b.WriteString("cluster0: " + yamlString(cfg.SourceURI) + "\n")
	b.WriteString("cluster1: " + yamlString(cfg.DestinationURI) + "\n")
	b.WriteString("logPath: " + yamlString(logDir) + "\n")
	b.WriteString("port: " + strconv.Itoa(cfg.Port) + "\n")
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// yamlString renders s as a double-quoted YAML scalar.
func yamlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
