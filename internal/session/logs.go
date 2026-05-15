package session

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// lastMongosyncError returns a human-readable reason from the most recent
// fatal or error entry in a mongosync log, or "" if none is found.
func lastMongosyncError(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry struct {
			Level   string `json:"level"`
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry.Level == "fatal" || entry.Level == "error" {
			if entry.Error.Message != "" {
				return entry.Error.Message
			}
			return entry.Message
		}
	}
	return ""
}

// initHintFromLog explains why mongosync is in the INITIALIZING state. It
// returns a human-readable message and whether the situation indicates a
// problem (true) rather than expected progress (false).
func initHintFromLog(logPath string) (message string, problem bool) {
	f, err := os.Open(logPath)
	if err != nil {
		return "", false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false
	}
	// Only the tail of the log is relevant; bound the read.
	const window = 64 * 1024
	if info.Size() > window {
		if _, err := f.Seek(info.Size()-window, io.SeekStart); err != nil {
			return "", false
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		switch {
		case strings.Contains(line, "transient error"):
			var e struct {
				OperationDescription string `json:"operationDescription"`
				HandledError         struct {
					Message string `json:"message"`
				} `json:"handledError"`
			}
			if json.Unmarshal([]byte(line), &e) != nil {
				continue
			}
			if e.OperationDescription != "" {
				return e.OperationDescription, true
			}
			return e.HandledError.Message, true
		case strings.Contains(line, "restarted mongosync after a pause or crash"):
			return "mongosync found state from a previous run on the destination " +
				"cluster and is resuming it. It waits up to ~2 minutes before it " +
				"accepts a new migration. To run a fresh migration, stop this " +
				"session and point it at clean clusters.", false
		}
	}
	return "", false
}
