// Package session ties together the mongosync binary, a supervised local
// process and the control client into a single migration session. Only one
// session exists at a time (local or attached to a remote instance).
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

	"mongosync-ui/internal/binary"
	"mongosync-ui/internal/client"
	"mongosync-ui/internal/process"
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
}

// State is the persisted session state.
type State struct {
	Mode       string          `json:"mode"` // none | local | remote
	APIBaseURL string          `json:"apiBaseUrl,omitempty"`
	PID        int             `json:"pid,omitempty"`
	Config     MigrationConfig `json:"config"`
	StartedAt  string          `json:"startedAt,omitempty"`
}

// Session coordinates the migration lifecycle.
type Session struct {
	workdir string
	bin     *binary.Manager
	proc    *process.Manager

	mu    sync.Mutex
	state State
}

const (
	ModeNone   = "none"
	ModeLocal  = "local"
	ModeRemote = "remote"
)

// New builds a Session for the given working directory and binary manager,
// restoring any previous state from disk.
func New(workdir string, bin *binary.Manager) *Session {
	s := &Session{
		workdir: workdir,
		bin:     bin,
		proc:    process.New(),
		state:   State{Mode: ModeNone},
	}
	s.restore()
	return s
}

func (s *Session) configDir() string  { return filepath.Join(s.workdir, "config") }
func (s *Session) logsDir() string     { return filepath.Join(s.workdir, "logs") }
func (s *Session) configPath() string  { return filepath.Join(s.configDir(), "mongosync.yaml") }
func (s *Session) procLogPath() string { return filepath.Join(s.logsDir(), "process.log") }
func (s *Session) statePath() string   { return filepath.Join(s.workdir, "state.json") }

// ProcLogPath is the file capturing the mongosync process stdout/stderr.
func (s *Session) ProcLogPath() string { return s.procLogPath() }

// MongosyncLogPath is the structured log file written by mongosync itself.
func (s *Session) MongosyncLogPath() string {
	return filepath.Join(s.logsDir(), "mongosync", "mongosync.log")
}

// restore loads state.json. A previously local session whose process is no
// longer supervised is downgraded to a remote attachment if still reachable,
// otherwise reset to none.
func (s *Session) restore() {
	raw, err := os.ReadFile(s.statePath())
	if err != nil {
		return
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return
	}
	if st.Mode == ModeNone || st.APIBaseURL == "" {
		s.state = State{Mode: ModeNone}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.New(st.APIBaseURL).Ping(ctx); err != nil {
		s.state = State{Mode: ModeNone}
		_ = os.Remove(s.statePath())
		return
	}
	// The child process is no longer ours to supervise after a UI restart;
	// keep monitoring it as a remote attachment.
	st.Mode = ModeRemote
	st.PID = 0
	s.state = st
}

func (s *Session) persist() {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.statePath(), raw, 0o600)
}

// Snapshot returns a copy of the current state.
func (s *Session) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state
	if st.Mode == ModeLocal {
		st.PID = s.proc.PID()
	}
	return st
}

// Running reports whether a session is active (local or remote).
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Mode == ModeLocal {
		return s.proc.Running()
	}
	return s.state.Mode == ModeRemote
}

// Client returns a control client for the active session, or nil when idle.
func (s *Session) Client() *client.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Mode == ModeNone || s.state.APIBaseURL == "" {
		return nil
	}
	return client.New(s.state.APIBaseURL)
}

// StartLocal writes a mongosync config file, launches the binary and waits for
// its API to become reachable.
func (s *Session) StartLocal(cfg MigrationConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Mode != ModeNone {
		return fmt.Errorf("a session is already active; stop it first")
	}
	if !s.bin.Installed() {
		return fmt.Errorf("the mongosync binary is not installed yet")
	}
	if strings.TrimSpace(cfg.SourceURI) == "" || strings.TrimSpace(cfg.DestinationURI) == "" {
		return fmt.Errorf("source and destination connection strings are required")
	}
	if cfg.Port == 0 {
		cfg.Port = 27182
	}

	if err := os.MkdirAll(s.configDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.logsDir(), 0o755); err != nil {
		return err
	}
	if err := writeConfigYAML(s.configPath(), cfg, s.logsDir()); err != nil {
		return fmt.Errorf("write mongosync config: %w", err)
	}

	if err := s.proc.Start(s.bin.BinaryPath(), s.configPath(), s.procLogPath()); err != nil {
		return fmt.Errorf("launch mongosync: %w", err)
	}

	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	if err := client.New(apiURL).WaitReady(60 * time.Second); err != nil {
		_ = s.proc.Stop()
		return err
	}

	s.state = State{
		Mode:       ModeLocal,
		APIBaseURL: apiURL,
		PID:        s.proc.PID(),
		Config:     cfg,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	s.persist()
	return nil
}

// AttachRemote connects the UI to an already-running mongosync instance.
func (s *Session) AttachRemote(apiURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Mode != ModeNone {
		return fmt.Errorf("a session is already active; stop it first")
	}
	apiURL = normalizeURL(apiURL)
	if apiURL == "" {
		return fmt.Errorf("a mongosync API URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.New(apiURL).Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach mongosync at %s: %w", apiURL, err)
	}

	s.state = State{
		Mode:       ModeRemote,
		APIBaseURL: apiURL,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	s.persist()
	return nil
}

// Stop ends the session, terminating the local process when applicable.
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	if s.state.Mode == ModeLocal {
		err = s.proc.Stop()
	}
	s.state = State{Mode: ModeNone}
	_ = os.Remove(s.statePath())
	return err
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

// writeConfigYAML emits a minimal mongosync configuration file. The file holds
// connection strings with credentials, so it is written with 0600 permissions.
func writeConfigYAML(path string, cfg MigrationConfig, logsDir string) error {
	var b strings.Builder
	b.WriteString("# Generated by mongosync-ui — do not edit while a session is running.\n")
	b.WriteString("cluster0: " + yamlString(cfg.SourceURI) + "\n")
	b.WriteString("cluster1: " + yamlString(cfg.DestinationURI) + "\n")
	b.WriteString("logPath: " + yamlString(logsDir) + "\n")
	b.WriteString("port: " + strconv.Itoa(cfg.Port) + "\n")
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// yamlString renders s as a double-quoted YAML scalar.
func yamlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
