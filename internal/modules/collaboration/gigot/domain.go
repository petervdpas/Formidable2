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

// Filesystem is the atomic-write surface Manager needs: SaveFile is temp+fsync+rename, so a crash never leaves a partial sync.json.
type Filesystem interface {
	SaveFile(path string, content string) error
}

// TrackRecordRel is the context-relative path of the client-side ledger.
const TrackRecordRel = ".formidable/sync.json"

// rootAllowlist names repo-root files managed alongside templates/ + storage/, so a fresh clone receives README and .gitignore.
var rootAllowlist = map[string]struct{}{
	"README.md":  {},
	".gitignore": {},
}

// Manager is the transport-neutral gigot backend; HTTP methods take an explicit Connection so callers swap servers/tokens without re-init.
type Manager struct {
	fs     Filesystem
	client *http.Client
}

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*Manager)

// WithHTTPClient swaps the default http.Client.
func WithHTTPClient(c *http.Client) ManagerOption {
	return func(m *Manager) {
		if c != nil {
			m.client = c
		}
	}
}

// NewManager builds a Manager bound to fs (required: ledger writes always go through it for atomicity).
func NewManager(fs Filesystem, opts ...ManagerOption) *Manager {
	m := &Manager{
		fs: fs,
		// A multi-file push uploads every changed file in one commit body, so
		// the old 30s ceiling could trip on a large context. 120s leaves the
		// server's own 60s commit deadline room to return a clean error first.
		client: &http.Client{Timeout: 120 * time.Second},
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

// GitBlobSha computes a blob's git SHA1: SHA1("blob " + len + "\0" + content), matching git's tree-entry hash.
func GitBlobSha(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// IsFormidablePath reports whether a repo-relative path is Formidable-managed (templates/, relations/, storage/, or an allowlisted root file).
// Rejects ".." traversal so callers can pass server tree paths without re-validating.
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
	if strings.HasPrefix(clean, "relations/") {
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

// EmptyTrackRecord returns a zero-value track record with a non-nil Files map.
func EmptyTrackRecord() TrackRecord {
	return TrackRecord{Files: map[string]string{}}
}

// TrackRecordPath returns the absolute path of the client-side ledger for a context folder.
func TrackRecordPath(contextFolder string) string {
	return filepath.Join(filepath.Clean(contextFolder), filepath.FromSlash(TrackRecordRel))
}

// ReadTrackRecord loads the ledger at contextFolder, returning EmptyTrackRecord (no error) when missing/corrupt so a fresh checkout doesn't break sync.
func (m *Manager) ReadTrackRecord(contextFolder string) TrackRecord {
	return ReadTrackRecord(contextFolder)
}

// ReadTrackRecord is the package-level form for callers that don't hold a Manager.
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

// WriteTrackRecord persists the ledger to <context>/.formidable/sync.json via the atomic-write Filesystem, pretty-printed for diffability.
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

// CollectFormidableFiles returns every managed file (templates/ + relations/ flat *.yaml, storage/ recursive, allowlisted root files), each with its bytes + git blob SHA.
func CollectFormidableFiles(contextFolder string) ([]LocalFile, error) {
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	root := filepath.Clean(contextFolder)
	var out []LocalFile

	if err := collectFlatYAML(root, "templates", &out); err != nil {
		return nil, err
	}
	// relations/ nests a self/<template>.yaml mirror for self-relations, so it
	// is walked recursively; IsFormidablePath already matches relations/ at any
	// depth, and a flat walk here would push the forward half but never the
	// self/ mirror, leaving the remote half of a self-relation behind.
	if err := collectYAMLTree(root, "relations", &out); err != nil {
		return nil, err
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

// collectFlatYAML appends every top-level *.yaml file under <root>/<sub> to out,
// tagged with the "<sub>/<name>" repo-relative path. templates/ and relations/
// are both flat directories of yaml documents; a missing directory is not an error.
func collectFlatYAML(root, sub string, out *[]LocalFile) error {
	dir := filepath.Join(root, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		f, err := readLocalFile(filepath.Join(dir, name), sub+"/"+name)
		if err != nil {
			return err
		}
		*out = append(*out, f)
	}
	return nil
}

// collectYAMLTree appends every *.yaml file under <root>/<sub> at any depth,
// tagged with its repo-relative path. Used for relations/, which (unlike the
// flat templates/ directory) nests a self/ subfolder of self-relation mirrors.
func collectYAMLTree(root, sub string, out *[]LocalFile) error {
	base := filepath.Join(root, sub)
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return walkYAMLTree(base, sub, out)
}

func walkYAMLTree(absDir, relDir string, out *[]LocalFile) error {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		abs := filepath.Join(absDir, e.Name())
		rel := relDir + "/" + e.Name()
		if e.IsDir() {
			if err := walkYAMLTree(abs, rel, out); err != nil {
				return err
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yaml") {
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

// DiffResult holds files whose SHA changed (Changed) and managed paths the ledger remembers but disk lost (Deleted).
type DiffResult struct {
	Changed []LocalFile
	Deleted []string
}

// DiffAgainstRecord compares the walker output to the track-record.
// On first sync (rec.Version empty) Deleted is suppressed: the seeded ledger holds every server path and we must not nuke content pull will fetch next.
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
