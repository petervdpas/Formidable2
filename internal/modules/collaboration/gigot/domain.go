package gigot

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Filesystem is the narrow surface Manager needs for atomic writes.
// system.Manager satisfies it directly. Local interface so this
// package compiles standalone; the composition root injects the real
// implementation. Atomic on success — temp file + fsync + rename —
// so a crash mid-write never leaves a partial sync.json on disk.
type Filesystem interface {
	SaveFile(path string, content string) error
}

// TrackRecordRel is the .formidable/-relative path of the client-side
// ledger inside a context folder. Kept as a public const so tests and
// callers don't redefine the string.
const TrackRecordRel = ".formidable/sync.json"

// rootAllowlist names the files at the repo root that Formidable
// treats as managed in addition to templates/ + storage/. Seeded by
// the gigot scaffolder; pushed and pulled like first-class content
// so a fresh clone actually receives the README and .gitignore.
var rootAllowlist = map[string]struct{}{
	"README.md":  {},
	".gitignore": {},
}

// Manager is the transport-neutral gigot backend. HTTP-bound methods
// take an explicit Connection so callers can swap servers/tokens
// without a stateful re-init. Pure helpers (file walk, blob hash,
// track-record I/O) live here too so the Service shim stays thin.
type Manager struct {
	fs     Filesystem
	client *http.Client
}

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*Manager)

// WithHTTPClient swaps the default http.Client — used by tests to
// inject an httptest.Server's transport or a recording RoundTripper.
func WithHTTPClient(c *http.Client) ManagerOption {
	return func(m *Manager) {
		if c != nil {
			m.client = c
		}
	}
}

// NewManager builds a Manager bound to the given filesystem. The
// Filesystem is required (track-record writes always go through it,
// per the atomic-writes rule). HTTP client defaults to one with a
// reasonable timeout; tests override via WithHTTPClient.
func NewManager(fs Filesystem, opts ...ManagerOption) *Manager {
	m := &Manager{
		fs:     fs,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func validateConn(conn Connection, requireRepo bool) error {
	if conn.BaseURL == "" {
		return ErrMissingBaseURL
	}
	if requireRepo && conn.RepoName == "" {
		return ErrMissingRepo
	}
	if conn.Token == "" {
		return ErrMissingToken
	}
	return nil
}

// ── Pure helpers (fully implemented + tested) ───────────────────────

// GitBlobSha computes the git blob SHA1 of the given bytes — the same
// hash git assigns to a blob entry in a tree. Formula:
// SHA1("blob " + len + "\0" + content). Lets the client compare local
// bytes against a TreeEntry.Blob without downloading the blob.
func GitBlobSha(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// IsFormidablePath classifies a repo-relative path as Formidable-managed
// content. True for templates/<name>.yaml (any depth — server is the
// source of truth for layout), storage/** at any depth, and the
// allowlisted root files (README.md, .gitignore). False for everything
// else (including .formidable/ which is owned by the server scaffold +
// the local ledger).
//
// Rejects ".." traversal segments outright so callers can pass paths
// straight from a server tree response without re-validating.
func IsFormidablePath(repoRelPath string) bool {
	if repoRelPath == "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(repoRelPath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	if strings.HasPrefix(clean, "templates/") {
		return true
	}
	if strings.HasPrefix(clean, "storage/") {
		return true
	}
	if !strings.Contains(clean, "/") {
		if _, ok := rootAllowlist[clean]; ok {
			return true
		}
	}
	return false
}

// EmptyTrackRecord returns a zero-value track record with a non-nil
// Files map. Callers can write to .Files without a nil-check; serialise
// produces an empty object rather than null.
func EmptyTrackRecord() TrackRecord {
	return TrackRecord{Files: map[string]string{}}
}

// TrackRecordPath returns the absolute path of the client-side ledger
// for a context folder. Stable across processes so the same context
// always resolves to the same file.
func TrackRecordPath(contextFolder string) string {
	return filepath.Join(filepath.Clean(contextFolder), filepath.FromSlash(TrackRecordRel))
}

// ReadTrackRecord loads the ledger at contextFolder. Returns
// EmptyTrackRecord() (no error) when the file is missing, unreadable,
// or syntactically corrupt — so a fresh checkout or a partial write
// from an older client doesn't break sync. Callers can treat the
// result as authoritative on its own.
func (m *Manager) ReadTrackRecord(contextFolder string) TrackRecord {
	return ReadTrackRecord(contextFolder)
}

// ReadTrackRecord is the package-level form for callers that don't
// hold a Manager (tests, scripts). The method form delegates here.
func ReadTrackRecord(contextFolder string) TrackRecord {
	if contextFolder == "" {
		return EmptyTrackRecord()
	}
	raw, err := os.ReadFile(TrackRecordPath(contextFolder))
	if err != nil {
		return EmptyTrackRecord()
	}
	var rec TrackRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return EmptyTrackRecord()
	}
	if rec.Files == nil {
		rec.Files = map[string]string{}
	}
	return rec
}

// WriteTrackRecord persists the ledger to <context>/.formidable/sync.json
// via the injected atomic-write Filesystem. Pretty-printed so a human
// can diff two snapshots; key order is map-iteration order in Go 1.18+
// (sorted alphabetically), which matches what the old client emitted.
func (m *Manager) WriteTrackRecord(contextFolder string, rec TrackRecord) error {
	if m.fs == nil {
		return fmt.Errorf("gigot: filesystem not configured")
	}
	if contextFolder == "" {
		return ErrMissingContext
	}
	target := TrackRecordPath(contextFolder)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if rec.Files == nil {
		rec.Files = map[string]string{}
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return m.fs.SaveFile(target, string(raw))
}

// CollectFormidableFiles walks the context folder and returns every
// file under templates/ (flat *.yaml only) + storage/ (recursive) +
// the allowlisted root files. Each entry carries the bytes + git blob
// SHA so callers can diff against the ledger without re-hashing.
// Skips .formidable/ implicitly by only touching the listed locations.
func CollectFormidableFiles(contextFolder string) ([]LocalFile, error) {
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	root := filepath.Clean(contextFolder)
	var out []LocalFile

	templatesDir := filepath.Join(root, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		f, err := readLocalFile(filepath.Join(templatesDir, name), "templates/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}

	storageDir := filepath.Join(root, "storage")
	if _, err := os.Stat(storageDir); err == nil {
		if err := walkStorage(storageDir, "storage", &out); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	for name := range rootAllowlist {
		abs := filepath.Join(root, name)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		f, err := readLocalFile(abs, name)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func walkStorage(absDir, relDir string, out *[]LocalFile) error {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		abs := filepath.Join(absDir, e.Name())
		rel := relDir + "/" + e.Name()
		if e.IsDir() {
			if err := walkStorage(abs, rel, out); err != nil {
				return err
			}
			continue
		}
		f, err := readLocalFile(abs, rel)
		if err != nil {
			return err
		}
		*out = append(*out, f)
	}
	return nil
}

func readLocalFile(abs, repoRel string) (LocalFile, error) {
	buf, err := os.ReadFile(abs)
	if err != nil {
		return LocalFile{}, err
	}
	return LocalFile{
		Path:  repoRel,
		Bytes: buf,
		Sha:   GitBlobSha(buf),
	}, nil
}

// DiffResult is the outcome of comparing the local walker output
// against the track-record. Changed lists files whose local SHA
// differs from the ledger (new puts); Deleted lists managed paths the
// ledger remembers but no longer exist on disk. On a first sync
// (rec.Version == ""), Deleted is empty regardless of the ledger
// contents — see DiffAgainstRecord for the policy.
type DiffResult struct {
	Changed []LocalFile
	Deleted []string
}

// DiffAgainstRecord compares the walker output to the track-record.
// Returns the list of files whose SHA differs from rec.Files plus the
// list of managed paths the ledger remembers but disk no longer has.
// On first sync (rec.Version empty), Deleted is suppressed — a freshly
// seeded ledger contains every server path and we don't want to nuke
// content that pullLocal will fetch in the next step.
func DiffAgainstRecord(local []LocalFile, rec TrackRecord) DiffResult {
	out := DiffResult{}
	localPaths := make(map[string]struct{}, len(local))
	for _, f := range local {
		localPaths[f.Path] = struct{}{}
		if rec.Files[f.Path] != f.Sha {
			out.Changed = append(out.Changed, f)
		}
	}
	if rec.Version == "" {
		return out
	}
	for p := range rec.Files {
		if !IsFormidablePath(p) {
			continue
		}
		if _, ok := localPaths[p]; !ok {
			out.Deleted = append(out.Deleted, p)
		}
	}
	sort.Strings(out.Deleted)
	return out
}
