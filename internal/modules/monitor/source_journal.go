package monitor

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// journalContext yields the active context folder (the absolute path
// containing .changes.log).
type journalContext interface {
	ContextFolder() string
}

// journalFS is the filesystem surface JournalSource needs.
type journalFS interface {
	FileExists(path string) bool
	LoadFile(path string) (string, error)
}

// JournalSource projects entries from <context>/.changes.log into
// Events. The op dim distinguishes mutation kinds (create/update/
// delete/baseline/sync); template is derived from path so consumers
// can group by template stem without re-parsing.
type JournalSource struct {
	ctxProvider journalContext
	fs          journalFS
}

// NewJournalSource builds a source. Both deps are required; nil
// arguments produce a Source that always returns no events (safer
// than panicking at query time).
func NewJournalSource(ctx journalContext, fs journalFS) *JournalSource {
	return &JournalSource{ctxProvider: ctx, fs: fs}
}

func (s *JournalSource) Name() string { return "journal" }
func (s *JournalSource) Kind() string { return "mutation" }
func (s *JournalSource) Dims() []string {
	return []string{"op", "backend", "path", "template"}
}

// Events scans .changes.log line-by-line, projects each valid entry,
// and clips to [from, to). Malformed lines are skipped silently -
// matches the journal module's own tolerance for partial corruption.
func (s *JournalSource) Events(from, to time.Time) []Event {
	if s.ctxProvider == nil || s.fs == nil {
		return nil
	}
	ctx := s.ctxProvider.ContextFolder()
	if ctx == "" {
		return nil
	}
	logPath := filepath.Join(ctx, ".changes.log")
	if !s.fs.FileExists(logPath) {
		return nil
	}
	raw, err := s.fs.LoadFile(logPath)
	if err != nil {
		return nil
	}
	out := make([]Event, 0, 64)
	for line := range strings.SplitSeq(raw, "\n") {
		ev, ok := parseJournalLine(line)
		if !ok {
			continue
		}
		if !from.IsZero() && ev.Ts.Before(from) {
			continue
		}
		if !to.IsZero() && !ev.Ts.Before(to) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// parseJournalLine reads one JSONL line into an Event. Returns
// (zero, false) for malformed/empty lines so callers can scan a
// partially-corrupted log.
func parseJournalLine(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Event{}, false
	}
	var entry struct {
		Ts      string `json:"ts"`
		Op      string `json:"op"`
		Path    string `json:"path,omitempty"`
		Backend string `json:"backend,omitempty"`
		Version string `json:"version,omitempty"`
		Pushed  int    `json:"pushed,omitempty"`
		Pulled  int    `json:"pulled,omitempty"`
		Bytes   int64  `json:"bytes,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return Event{}, false
	}
	if entry.Ts == "" || entry.Op == "" {
		return Event{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, entry.Ts)
	if err != nil {
		return Event{}, false
	}

	dims := map[string]string{"op": entry.Op}
	if entry.Backend != "" {
		dims["backend"] = entry.Backend
	}
	if entry.Path != "" {
		dims["path"] = entry.Path
		if tpl := templateFromPath(entry.Path); tpl != "" {
			dims["template"] = tpl
		}
	}
	return Event{
		Ts:    ts,
		Kind:  "mutation",
		Dims:  dims,
		Value: 1,
	}, true
}

// templateFromPath derives the template stem from a journal-tracked path:
//
//	"templates/recepten.yaml"        → "recepten"
//	"storage/recepten/foo.meta.json" → "recepten"
//	"templates/" / unknown prefix    → ""
func templateFromPath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) < 2 || parts[1] == "" {
		return ""
	}
	switch parts[0] {
	case "templates":
		return strings.TrimSuffix(parts[1], ".yaml")
	case "storage":
		return parts[1]
	}
	return ""
}
