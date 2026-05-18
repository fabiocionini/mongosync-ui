// Package server exposes the mongosync-ui REST API and serves the embedded
// single-page application.
package server

import (
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/fabiocionini/mongosync-ui/internal/binary"
	"github.com/fabiocionini/mongosync-ui/internal/session"
)

// Server wires HTTP handlers to the session and binary managers.
type Server struct {
	sess    *session.Session
	bin     *binary.Manager
	web     fs.FS
	version string
}

// New constructs a Server.
func New(sess *session.Session, bin *binary.Manager, web fs.FS, version string) *Server {
	return &Server{sess: sess, bin: bin, web: web, version: version}
}

// Handler returns the root HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/binary/status", s.handleBinaryStatus)
	mux.HandleFunc("GET /api/binary/versions", s.handleBinaryVersions)
	mux.HandleFunc("POST /api/binary/install", s.handleBinaryInstall)

	mux.HandleFunc("POST /api/analyze", s.handleAnalyze)

	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSessionByID)
	mux.HandleFunc("GET /api/sessions/{id}/logs", s.handleSessionLogs)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)

	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/session/local", s.handleStartLocal)
	mux.HandleFunc("POST /api/session/remote", s.handleAttachRemote)
	mux.HandleFunc("DELETE /api/session", s.handleStopSession)

	mux.HandleFunc("GET /api/progress", s.handleProgress)
	mux.HandleFunc("POST /api/start", s.handleStart)
	mux.HandleFunc("POST /api/pause", s.handleAction("pause"))
	mux.HandleFunc("POST /api/resume", s.handleAction("resume"))
	mux.HandleFunc("POST /api/commit", s.handleAction("commit"))
	mux.HandleFunc("POST /api/reverse", s.handleAction("reverse"))

	mux.Handle("/", s.spaHandler())

	return logMiddleware(mux)
}

// spaHandler serves embedded assets and falls back to index.html so that
// client-side routing works.
func (s *Server) spaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if name == "." || name == "" {
			name = "index.html"
		}

		f, err := s.web.Open(name)
		if err != nil {
			name = "index.html"
			f, err = s.web.Open(name)
			if err != nil {
				http.Error(w, "web UI not built", http.StatusInternalServerError)
				return
			}
		}
		defer f.Close()

		if info, err := f.Stat(); err == nil && info.IsDir() {
			f.Close()
			name = "index.html"
			f, err = s.web.Open(name)
			if err != nil {
				http.Error(w, "web UI not built", http.StatusInternalServerError)
				return
			}
			defer f.Close()
		}

		if ctype := mime.TypeByExtension(path.Ext(name)); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		if seeker, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, name, time.Time{}, seeker)
			return
		}
		_, _ = io.Copy(w, f)
	}
}

// logMiddleware logs mutating API requests. Read-only GET endpoints are
// skipped — the UI polls them every couple of seconds, which would otherwise
// flood the log with noise.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.Method != http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
