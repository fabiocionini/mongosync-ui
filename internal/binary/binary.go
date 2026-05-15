// Package binary downloads, verifies and manages the official mongosync
// binary, storing it inside the application's working directory.
package binary

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// FeedURL is the official MongoDB download manifest for mongosync.
const FeedURL = "https://downloads.mongodb.org/tools/mongosync/full.json"

// archive describes a single downloadable package within the feed.
type archive struct {
	URL    string `json:"url"`
	Sha256 string `json:"sha256"`
}

type download struct {
	Name    string  `json:"name"`
	Arch    string  `json:"arch"`
	Archive archive `json:"archive"`
}

type feedVersion struct {
	Version   string     `json:"version"`
	Downloads []download `json:"downloads"`
}

type feed struct {
	Versions []feedVersion `json:"versions"`
}

// Status is the observable state of the managed binary.
type Status struct {
	State    string `json:"state"` // absent | downloading | extracting | installed | error
	Version  string `json:"version,omitempty"`
	Progress int    `json:"progress"` // 0-100, meaningful while downloading
	Error    string `json:"error,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Manager owns the mongosync binary lifecycle for one working directory.
type Manager struct {
	binDir string
	mu     sync.Mutex
	status Status
}

// NewManager returns a Manager rooted at binDir and detects an existing install.
func NewManager(binDir string) *Manager {
	m := &Manager{binDir: binDir, status: Status{State: "absent"}}
	m.detect()
	return m
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "mongosync.exe"
	}
	return "mongosync"
}

// BinaryPath is the absolute path where the managed binary lives.
func (m *Manager) BinaryPath() string { return filepath.Join(m.binDir, binaryName()) }

func (m *Manager) versionFile() string { return filepath.Join(m.binDir, "version.txt") }

// Installed reports whether the mongosync binary is present on disk.
func (m *Manager) Installed() bool {
	_, err := os.Stat(m.BinaryPath())
	return err == nil
}

// Status returns a snapshot of the current binary state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) setStatus(s Status) {
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
}

func (m *Manager) detect() {
	if !m.Installed() {
		return
	}
	version := ""
	if b, err := os.ReadFile(m.versionFile()); err == nil {
		version = strings.TrimSpace(string(b))
	}
	m.status = Status{State: "installed", Version: version, Progress: 100, Path: m.BinaryPath()}
}

func httpClient() *http.Client { return &http.Client{Timeout: 0} }

func fetchFeed() (*feed, error) {
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(FeedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch download feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download feed returned HTTP %d", resp.StatusCode)
	}
	var f feed
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode download feed: %w", err)
	}
	return &f, nil
}

func isPreview(v string) bool { return strings.Contains(strings.ToLower(v), "preview") }

// Versions returns the stable mongosync versions installable on the current
// platform, newest first. Versions without a build for this OS/arch are
// omitted — for example mongosync ships no Windows build after 1.5.0.
func Versions() ([]string, error) {
	name, arch, err := platformTarget()
	if err != nil {
		return nil, err
	}
	f, err := fetchFeed()
	if err != nil {
		return nil, err
	}
	var out []string
	for i := len(f.Versions) - 1; i >= 0; i-- {
		v := f.Versions[i]
		if isPreview(v.Version) {
			continue
		}
		for _, d := range v.Downloads {
			if d.Name == name && d.Arch == arch {
				out = append(out, v.Version)
				break
			}
		}
	}
	return out, nil
}

// platformTarget maps the current OS/arch onto the feed's naming scheme.
func platformTarget() (name, arch string, err error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "macos-arm", "arm64", nil
	case "darwin/amd64":
		return "macos", "x86_64", nil
	case "linux/amd64":
		return "ubuntu2204", "x86_64", nil
	case "linux/arm64":
		return "amazon2023-arm64", "arm64", nil
	case "windows/amd64":
		return "windows", "x86_64", nil
	}
	return "", "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

// resolve picks the download entry for this platform. An empty version selects
// the newest stable release that has a build for the current OS/arch.
func resolve(f *feed, version string) (download, string, error) {
	name, arch, err := platformTarget()
	if err != nil {
		return download{}, "", err
	}
	find := func(v *feedVersion) (download, bool) {
		for _, d := range v.Downloads {
			if d.Name == name && d.Arch == arch {
				return d, true
			}
		}
		return download{}, false
	}

	if version != "" {
		for i := range f.Versions {
			if f.Versions[i].Version != version {
				continue
			}
			if d, ok := find(&f.Versions[i]); ok {
				return d, version, nil
			}
			return download{}, "", fmt.Errorf("mongosync %s has no build for %s/%s", version, name, arch)
		}
		return download{}, "", fmt.Errorf("mongosync version %q not found in feed", version)
	}

	for i := len(f.Versions) - 1; i >= 0; i-- {
		v := &f.Versions[i]
		if isPreview(v.Version) {
			continue
		}
		if d, ok := find(v); ok {
			return d, v.Version, nil
		}
	}
	return download{}, "", fmt.Errorf("no mongosync build available for %s/%s", name, arch)
}

// Install downloads the requested mongosync version in the background. An
// empty version selects the newest stable release. Progress is observable
// through Status.
func (m *Manager) Install(version string) {
	go func() {
		if err := m.install(version); err != nil {
			s := m.Status()
			m.setStatus(Status{State: "error", Version: s.Version, Error: err.Error()})
		}
	}()
}

func (m *Manager) install(version string) error {
	m.setStatus(Status{State: "downloading", Version: version, Progress: 0})

	f, err := fetchFeed()
	if err != nil {
		return err
	}
	dl, resolvedVer, err := resolve(f, version)
	if err != nil {
		return err
	}
	m.setStatus(Status{State: "downloading", Version: resolvedVer, Progress: 0})

	if err := os.MkdirAll(m.binDir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(m.binDir, "mongosync-download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := m.fetchArchive(dl, resolvedVer, tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	m.setStatus(Status{State: "extracting", Version: resolvedVer, Progress: 100})
	if err := extractBinary(tmpPath, dl.Archive.URL, m.BinaryPath()); err != nil {
		return err
	}
	if err := os.WriteFile(m.versionFile(), []byte(resolvedVer), 0o644); err != nil {
		return err
	}

	m.setStatus(Status{State: "installed", Version: resolvedVer, Progress: 100, Path: m.BinaryPath()})
	return nil
}

// fetchArchive streams the download to dst, verifies its SHA-256 checksum and
// reports progress through Status.
func (m *Manager) fetchArchive(dl download, version string, dst *os.File) error {
	resp, err := httpClient().Get(dl.Archive.URL)
	if err != nil {
		return fmt.Errorf("download mongosync: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download mongosync: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	total := resp.ContentLength
	pr := &progressReader{
		reader: io.TeeReader(resp.Body, hasher),
		total:  total,
		report: func(pct int) {
			m.setStatus(Status{State: "downloading", Version: version, Progress: pct})
		},
	}
	if _, err := io.Copy(dst, pr); err != nil {
		return fmt.Errorf("download mongosync: %w", err)
	}

	if want := strings.ToLower(dl.Archive.Sha256); want != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != want {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", want, got)
		}
	}
	return nil
}

// progressReader counts bytes and emits coarse progress updates.
type progressReader struct {
	reader  io.Reader
	total   int64
	read    int64
	lastPct int
	report  func(int)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.reader.Read(b)
	p.read += int64(n)
	if p.total > 0 {
		pct := int(p.read * 100 / p.total)
		if pct != p.lastPct {
			p.lastPct = pct
			p.report(pct)
		}
	}
	return n, err
}

// extractBinary pulls the mongosync executable out of the downloaded archive
// (zip for macOS/Windows, tar.gz for Linux) and writes it to target.
func extractBinary(archivePath, sourceURL, target string) error {
	want := binaryName()

	if strings.HasSuffix(sourceURL, ".zip") {
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return fmt.Errorf("open zip archive: %w", err)
		}
		defer zr.Close()
		for _, file := range zr.File {
			if file.FileInfo().IsDir() || filepath.Base(file.Name) != want {
				continue
			}
			rc, err := file.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeExecutable(target, rc)
		}
		return fmt.Errorf("%s not found inside archive", want)
	}

	fh, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return fmt.Errorf("open gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == want {
			return writeExecutable(target, tr)
		}
	}
	return fmt.Errorf("%s not found inside archive", want)
}

func writeExecutable(path string, r io.Reader) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return nil
}
