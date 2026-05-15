package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"mongosync-ui/internal/binary"
	"mongosync-ui/internal/client"
	"mongosync-ui/internal/session"
)

// writeJSON serializes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError emits a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleBinaryStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.bin.Status())
}

func (s *Server) handleBinaryVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := binary.Versions()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) handleBinaryInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.bin.Install(body.Version)
	writeJSON(w, http.StatusAccepted, s.bin.Status())
}

// sessionView is the combined session + binary state sent to the UI.
type sessionView struct {
	Mode          string                  `json:"mode"`
	APIBaseURL    string                  `json:"apiBaseUrl,omitempty"`
	PID           int                     `json:"pid,omitempty"`
	Running       bool                    `json:"running"`
	StartedAt     string                  `json:"startedAt,omitempty"`
	Config        session.MigrationConfig `json:"config"`
	Binary        binary.Status           `json:"binary"`
	ProcessExited bool                    `json:"processExited"`
	ExitReason    string                  `json:"exitReason,omitempty"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	st := s.sess.Snapshot()
	running := s.sess.Running()
	view := sessionView{
		Mode:       st.Mode,
		APIBaseURL: st.APIBaseURL,
		PID:        st.PID,
		Running:    running,
		StartedAt:  st.StartedAt,
		Config:     st.Config,
		Binary:     s.bin.Status(),
	}
	// A local session whose process is no longer running has crashed or been
	// terminated externally; surface the reason so the UI can explain it.
	if st.Mode == session.ModeLocal && !running {
		view.ProcessExited = true
		view.ExitReason = s.sess.LastMongosyncError()
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleStartLocal(w http.ResponseWriter, r *http.Request) {
	var cfg session.MigrationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.sess.StartLocal(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleSession(w, r)
}

func (s *Server) handleAttachRemote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.sess.AttachRemote(body.URL); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.handleSession(w, r)
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	if err := s.sess.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleSession(w, r)
}

// relay forwards a mongosync client response to the HTTP response writer.
func relay(w http.ResponseWriter, resp *client.Response, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	c := s.sess.Client()
	if c == nil {
		writeJSON(w, http.StatusOK, map[string]any{"mode": "none"})
		return
	}
	resp, err := c.Progress(r.Context())
	relay(w, resp, err)
}

// handleStart forwards a start request, defaulting the cluster mapping.
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	c := s.sess.Client()
	if c == nil {
		writeError(w, http.StatusConflict, "no active session")
		return
	}
	body := map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if _, ok := body["source"]; !ok {
		body["source"] = "cluster0"
	}
	if _, ok := body["destination"]; !ok {
		body["destination"] = "cluster1"
	}
	resp, err := c.Start(r.Context(), body)
	relay(w, resp, err)
}

// handleAction returns a handler that forwards a parameterless mongosync action.
func (s *Server) handleAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := s.sess.Client()
		if c == nil {
			writeError(w, http.StatusConflict, "no active session")
			return
		}
		var (
			resp *client.Response
			err  error
		)
		switch action {
		case "pause":
			resp, err = c.Pause(r.Context())
		case "resume":
			resp, err = c.Resume(r.Context())
		case "commit":
			resp, err = c.Commit(r.Context())
		case "reverse":
			resp, err = c.Reverse(r.Context())
		default:
			writeError(w, http.StatusNotFound, "unknown action")
			return
		}
		relay(w, resp, err)
	}
}

// handleLogs returns the tail of the mongosync log (local sessions only).
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	st := s.sess.Snapshot()
	if st.Mode != session.ModeLocal {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "lines": []string{}})
		return
	}
	limit := 400
	if q := r.URL.Query().Get("lines"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	lines := tailFile(s.sess.MongosyncLogPath(), limit)
	if len(lines) == 0 {
		lines = tailFile(s.sess.ProcLogPath(), limit)
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "lines": lines})
}

// tailFile returns the last n lines of a file, or nil if it cannot be read.
func tailFile(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	all := splitLines(string(data))
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if l := len(line); l > 0 && line[l-1] == '\r' {
				line = line[:l-1]
			}
			out = append(out, line)
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
