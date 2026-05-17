// Command mongosync-ui is a self-contained web UI for configuring, running,
// monitoring and committing MongoDB cluster-to-cluster migrations with
// mongosync. It can launch and manage a local mongosync binary or attach to
// an already-running remote instance.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fabiocionini/mongosync-ui/internal/binary"
	"github.com/fabiocionini/mongosync-ui/internal/server"
	"github.com/fabiocionini/mongosync-ui/internal/session"
	"github.com/fabiocionini/mongosync-ui/web"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	var (
		port    int
		workdir string
		open    bool
	)
	flag.IntVar(&port, "port", 8999, "port for the mongosync-ui web interface")
	flag.StringVar(&workdir, "workdir", defaultWorkdir(), "directory for the mongosync binary and session data")
	flag.BoolVar(&open, "open", true, "open the UI in a browser on startup")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("mongosync-ui", version)
		return
	}

	for _, dir := range []string{"bin", "sessions"} {
		if err := os.MkdirAll(filepath.Join(workdir, dir), 0o755); err != nil {
			log.Fatalf("create working directory: %v", err)
		}
	}

	bin := binary.NewManager(filepath.Join(workdir, "bin"))
	sess := session.New(workdir, bin)

	webFS, err := web.FS()
	if err != nil {
		log.Fatalf("load embedded web assets: %v", err)
	}

	srv := server.New(sess, bin, webFS, version)
	addr := fmt.Sprintf(":%d", port)
	url := fmt.Sprintf("http://localhost:%d", port)

	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}

	log.Printf("mongosync-ui %s", version)
	log.Printf("working directory: %s", workdir)
	log.Printf("web interface:     %s", url)

	if open {
		go func() {
			time.Sleep(600 * time.Millisecond)
			openBrowser(url)
		}()
	}

	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// defaultWorkdir returns ~/.mongosync-ui, falling back to ./mongosync-ui-data.
func defaultWorkdir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".mongosync-ui")
	}
	return "mongosync-ui-data"
}

// openBrowser best-effort opens the given URL in the system browser.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}
