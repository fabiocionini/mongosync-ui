package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/fabiocionini/mongosync-ui/internal/binary"
	"github.com/fabiocionini/mongosync-ui/internal/client"
	"github.com/fabiocionini/mongosync-ui/internal/session"
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

// --- binary ---------------------------------------------------------------

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

// --- sessions registry ----------------------------------------------------

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"records": s.sess.Records()})
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.sess.Record(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleSessionLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.sess.Record(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	limit := parseLines(r, 400)
	lines := tailFile(s.sess.MongosyncLogPath(id), limit)
	if len(lines) == 0 {
		lines = tailFile(s.sess.ProcLogPath(id), limit)
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "lines": lines})
}

// --- active session -------------------------------------------------------

// activeView is the active session enriched with live, non-persisted detail.
type activeView struct {
	Record          session.Record `json:"record"`
	InitHint        string         `json:"initHint,omitempty"`
	InitHintProblem bool           `json:"initHintProblem,omitempty"`
}

// sessionResponse is returned by GET /api/session.
type sessionResponse struct {
	Active *activeView   `json:"active"`
	Binary binary.Status `json:"binary"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	resp := sessionResponse{Binary: s.bin.Status()}
	if rec, ok := s.sess.Active(); ok {
		av := &activeView{Record: rec}
		if rec.Mode == session.ModeLocal {
			av.InitHint, av.InitHintProblem = s.sess.InitializingHint()
		}
		resp.Active = av
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStartLocal(w http.ResponseWriter, r *http.Request) {
	var cfg session.MigrationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := s.sess.StartLocal(cfg); err != nil {
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
	if _, err := s.sess.AttachRemote(body.URL); err != nil {
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

// --- mongosync control proxy ---------------------------------------------

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
	if err == nil && resp != nil {
		// Peek at the progress to keep the session record current.
		var pr struct {
			Progress struct {
				State              string `json:"state"`
				Info               string `json:"info"`
				TotalEventsApplied int64  `json:"totalEventsApplied"`
				CollectionCopy     struct {
					EstimatedTotalBytes  int64 `json:"estimatedTotalBytes"`
					EstimatedCopiedBytes int64 `json:"estimatedCopiedBytes"`
				} `json:"collectionCopy"`
				IndexBuilding struct {
					IndexesBuilt        int `json:"indexesBuilt"`
					TotalIndexesToBuild int `json:"totalIndexesToBuild"`
				} `json:"indexBuilding"`
				Verification struct {
					Destination struct {
						EstimatedDocumentCount int64 `json:"estimatedDocumentCount"`
						HashedDocumentCount    int64 `json:"hashedDocumentCount"`
						TotalCollectionCount   int   `json:"totalCollectionCount"`
						ScannedCollectionCount int   `json:"scannedCollectionCount"`
					} `json:"destination"`
				} `json:"verification"`
				DirectionMapping struct {
					Source      string `json:"Source"`
					Destination string `json:"Destination"`
				} `json:"directionMapping"`
			} `json:"progress"`
		}
		if json.Unmarshal(resp.Body, &pr) == nil {
			p := pr.Progress
			s.sess.UpdateProgress(
				p.State,
				p.DirectionMapping.Source,
				p.DirectionMapping.Destination,
				session.Summary{
					Phase:               p.Info,
					CopiedBytes:         p.CollectionCopy.EstimatedCopiedBytes,
					TotalBytes:          p.CollectionCopy.EstimatedTotalBytes,
					EventsApplied:       p.TotalEventsApplied,
					IndexesBuilt:        p.IndexBuilding.IndexesBuilt,
					TotalIndexes:        p.IndexBuilding.TotalIndexesToBuild,
					VerifiedDocuments:   p.Verification.Destination.HashedDocumentCount,
					EstimatedDocuments:  p.Verification.Destination.EstimatedDocumentCount,
					VerifiedCollections: p.Verification.Destination.ScannedCollectionCount,
					TotalCollections:    p.Verification.Destination.TotalCollectionCount,
				},
			)
		}
	}
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
	// Remember whether the session was started reversible — mongosync does not
	// report this, and the UI gates the Reverse control on it.
	if rev, ok := body["reversible"].(bool); ok {
		s.sess.SetReversible(rev)
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

// --- helpers --------------------------------------------------------------

// parseLines reads a bounded ?lines= query parameter.
func parseLines(r *http.Request, def int) int {
	if q := r.URL.Query().Get("lines"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 5000 {
			return n
		}
	}
	return def
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
