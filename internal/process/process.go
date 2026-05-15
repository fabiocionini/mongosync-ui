// Package process supervises a locally launched mongosync child process.
package process

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Manager starts, tracks and stops a single mongosync process.
type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	logFile *os.File
	running bool
	exitErr error
}

// New returns an idle process Manager.
func New() *Manager { return &Manager{} }

// Start launches `mongosync --config <configPath>`, redirecting the process
// stdout/stderr to procLogPath. It returns once the process has been spawned;
// callers should poll the mongosync API to confirm readiness.
func (m *Manager) Start(binPath, configPath, procLogPath string) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return errors.New("mongosync is already running")
	}
	m.mu.Unlock()

	logFile, err := os.Create(procLogPath)
	if err != nil {
		return err
	}

	cmd := exec.Command(binPath, "--config", configPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}

	m.mu.Lock()
	m.cmd = cmd
	m.logFile = logFile
	m.running = true
	m.exitErr = nil
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		logFile.Close()
		m.mu.Lock()
		m.running = false
		m.exitErr = err
		m.mu.Unlock()
	}()

	return nil
}

// Stop asks the process to terminate gracefully, escalating to a hard kill if
// it does not exit within the grace period.
func (m *Manager) Stop() error {
	m.mu.Lock()
	cmd := m.cmd
	running := m.running
	m.mu.Unlock()

	if !running || cmd == nil || cmd.Process == nil {
		return nil
	}

	_ = cmd.Process.Signal(os.Interrupt)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !m.Running() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return cmd.Process.Kill()
}

// Running reports whether the supervised process is currently alive.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// PID returns the process id, or 0 when nothing is running.
func (m *Manager) PID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil && m.running {
		return m.cmd.Process.Pid
	}
	return 0
}

// ExitError returns the error from the last process exit, if any.
func (m *Manager) ExitError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitErr
}
