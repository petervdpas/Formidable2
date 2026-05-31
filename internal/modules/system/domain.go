package system

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	mu      sync.RWMutex
	appRoot string
	log     *slog.Logger
	journal JournalEmitter
}

func NewManager(appRoot string, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{appRoot: appRoot, log: log}
}

func (m *Manager) SetJournal(j JournalEmitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journal = j
}

func (m *Manager) AppRoot() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.appRoot
}

func (m *Manager) SetAppRoot(root string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appRoot = root
}

func (m *Manager) JoinPath(segments ...string) string {
	if len(segments) == 0 {
		return m.AppRoot()
	}
	if filepath.IsAbs(segments[0]) {
		return filepath.Join(segments...)
	}
	return filepath.Join(append([]string{m.AppRoot()}, segments...)...)
}

// ResolveAbsolutePath cleans a user-typed path to absolute, independent of
// AppRoot. Empty stays empty; "~"/"~/sub" expand to home ("~someuser" is
// left as-is); relative resolves against the working dir.
func (m *Manager) ResolveAbsolutePath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(p)
}

// MakeAppRootRelative collapses an absolute path under AppRoot to "./<rel>"
// for portable config values. The root collapses to "."; paths outside
// AppRoot (and already-relative input) pass through unchanged, keeping the
// "value is either ./<sub> or absolute" invariant.
func (m *Manager) MakeAppRootRelative(p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	root := m.AppRoot()
	if root == "" {
		return p
	}
	rel, err := filepath.Rel(root, filepath.Clean(p))
	if err != nil {
		return p
	}
	// Rel returns ".." when p is outside root; relativize descendants only.
	if rel == "." {
		return "."
	}
	if strings.HasPrefix(rel, "..") {
		return p
	}
	return "./" + rel
}

// escapesRoot reports whether full resolves outside root via a ".." traversal.
func escapesRoot(root, full string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m *Manager) ResolvePath(segments ...string) string {
	joined := filepath.Join(segments...)
	if filepath.IsAbs(joined) {
		return filepath.Clean(joined)
	}
	abs, err := filepath.Abs(filepath.Join(m.AppRoot(), joined))
	if err != nil {
		return filepath.Clean(filepath.Join(m.AppRoot(), joined))
	}
	return abs
}

func (m *Manager) EnsureDirectory(path string) error {
	full := m.ResolvePath(path)
	return os.MkdirAll(full, 0o755)
}

func (m *Manager) FileExists(path string) bool {
	_, err := os.Stat(m.ResolvePath(path))
	return err == nil
}

func (m *Manager) IsDir(path string) bool {
	info, err := os.Stat(m.ResolvePath(path))
	return err == nil && info.IsDir()
}

// ListDir returns the entry names in path. A missing directory yields nil,
// not an error (callers treat "no files yet" as normal); real I/O errors
// bubble up. Order is filesystem-dependent.
func (m *Manager) ListDir(path string) ([]string, error) {
	full := m.ResolvePath(path)
	entries, err := os.ReadDir(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

func (m *Manager) LoadFile(path string) (string, error) {
	full := m.ResolvePath(path)
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Manager) SaveFile(path string, content string) error {
	full := m.ResolvePath(path)
	// A relative key must stay under AppRoot; reject ".." traversal. Absolute
	// paths are a deliberate caller choice and pass through.
	if !filepath.IsAbs(path) && escapesRoot(m.AppRoot(), full) {
		return errors.New("system: path escapes the app root: " + path)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	existed := fileExists(full)
	if err := atomicWriteFile(full, []byte(content), 0o644); err != nil {
		return err
	}
	op := "create"
	if existed {
		op = "update"
	}
	m.emit(op, full, map[string]any{"bytes": len(content)})
	return nil
}

// AppendFile appends content to path (creating it if missing). Emits no
// journal op of its own; journals control their own emission.
func (m *Manager) AppendFile(path string, content string) error {
	full := m.ResolvePath(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return err
	}
	return nil
}

func (m *Manager) DeleteFile(path string) error {
	full := m.ResolvePath(path)
	if !fileExists(full) {
		return nil
	}
	if err := os.Remove(full); err != nil {
		return err
	}
	m.emit("delete", full, nil)
	return nil
}

func (m *Manager) DeleteFolder(path string) error {
	full := m.ResolvePath(path)
	if !fileExists(full) {
		return nil
	}
	leaves, _ := m.WalkFiles(full)
	if err := os.RemoveAll(full); err != nil {
		return err
	}
	for _, leaf := range leaves {
		m.emit("delete", leaf, nil)
	}
	return nil
}

func (m *Manager) EmptyFolder(path string) error {
	full := m.ResolvePath(path)
	entries, err := os.ReadDir(full)
	if err != nil {
		return err
	}
	for _, e := range entries {
		entryPath := filepath.Join(full, e.Name())
		if e.IsDir() {
			leaves, _ := m.WalkFiles(entryPath)
			if err := os.RemoveAll(entryPath); err != nil {
				return err
			}
			for _, leaf := range leaves {
				m.emit("delete", leaf, nil)
			}
		} else {
			if err := os.Remove(entryPath); err != nil {
				return err
			}
			m.emit("delete", entryPath, nil)
		}
	}
	return nil
}

func (m *Manager) CopyFile(from, to string, overwrite bool) error {
	src := m.ResolvePath(from)
	dst := m.ResolvePath(to)
	if !overwrite && fileExists(dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	existed := fileExists(dst)

	if err := atomicWriteStream(dst, 0o644, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	}); err != nil {
		return err
	}

	op := "create"
	if existed {
		op = "update"
	}
	info, _ := os.Stat(dst)
	var bytes int64
	if info != nil {
		bytes = info.Size()
	}
	m.emit(op, dst, map[string]any{"bytes": bytes})
	return nil
}

func (m *Manager) CopyFolder(from, to string, overwrite bool) error {
	src := m.ResolvePath(from)
	dst := m.ResolvePath(to)
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !overwrite && fileExists(target) {
			return nil
		}
		return m.CopyFile(p, target, overwrite)
	})
}

func (m *Manager) ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(m.ResolvePath(dir))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func (m *Manager) ListFolders(dir string) ([]string, error) {
	entries, err := os.ReadDir(m.ResolvePath(dir))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func (m *Manager) ListDirectoryEntries(dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(m.ResolvePath(dir))
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{
			Name:        e.Name(),
			IsDirectory: e.IsDir(),
			IsFile:      !e.IsDir(),
		})
	}
	return out, nil
}

func (m *Manager) WalkFiles(dir string) ([]string, error) {
	full := m.ResolvePath(dir)
	var out []string
	err := filepath.WalkDir(full, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

func (m *Manager) ExecuteCommand(cmdline string) (string, error) {
	if strings.TrimSpace(cmdline) == "" {
		return "", errors.New("empty command")
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("cmd", "/C", cmdline)
	default:
		c = exec.Command("sh", "-c", cmdline)
	}
	out, err := c.CombinedOutput()
	return string(out), err
}

func (m *Manager) OpenExternal(target string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", target)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		c = exec.Command("xdg-open", target)
	}
	return c.Start()
}

func (m *Manager) ProxyFetchRemote(url string, opts FetchOptions) (*FetchResult, error) {
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodGet
	}
	timeout := opts.TimeoutSecs
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	var body io.Reader
	if opts.Body != "" {
		body = strings.NewReader(opts.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	if !opts.FollowRedirs {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	hdrs := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		hdrs[k] = strings.Join(v, ", ")
	}
	finalURL := url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return &FetchResult{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Headers:    hdrs,
		URL:        finalURL,
	}, nil
}

// emit forwards a filesystem mutation to the journal if one is wired.
func (m *Manager) emit(op, path string, meta map[string]any) {
	m.mu.RLock()
	j := m.journal
	m.mu.RUnlock()
	if j == nil {
		return
	}
	j.RecordOp(op, path, meta)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// guard: *Manager must satisfy FS.
var _ FS = (*Manager)(nil)
