package pdf

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"
)

// stateFilePath is the per-machine activation record. NOT in user.json:
// browser_bin is machine-specific and would break under gigot/git sync.
// See design/pdf-export.md "Activation persistence".
const stateFilePath = "config/.pdf-state.json"

// state is the on-disk shape. ActivatedAt is omitzero so the inactive
// default is a clean `{}`. ExportDir survives Deactivate. Field tags
// are stable; do not rename without a migration.
type state struct {
	BrowserBin  string    `json:"browser_bin,omitempty"`
	Source      Source    `json:"source,omitempty"`
	Version     string    `json:"version,omitempty"`
	ActivatedAt time.Time `json:"activated_at,omitzero"`
	ExportDir   string    `json:"export_dir,omitempty"`
}

// storeFS is the filesystem surface the pdf module needs. ListDir
// returns an empty slice for a missing directory.
type storeFS interface {
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	ListDir(path string) ([]string, error)
	ResolvePath(segments ...string) string
}

// store reads/writes pdf-state.json. Missing file, malformed JSON, or
// nil fs degrade to "inactive" rather than blocking startup.
type store struct {
	fs  storeFS
	log *slog.Logger
}

// Load returns the persisted state, or the inactive zero value for a
// missing file / malformed JSON / nil fs. Real I/O errors bubble up.
func (s *store) Load() (state, error) {
	if s == nil || s.fs == nil {
		return state{Source: SourceUnset}, nil
	}
	raw, err := s.fs.LoadFile(stateFilePath)
	if err != nil {
		if isMissingErr(err) {
			return state{Source: SourceUnset}, nil
		}
		return state{Source: SourceUnset}, err
	}
	var st state
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		if s.log != nil {
			s.log.Warn("pdf: state file malformed; ignoring", "path", stateFilePath, "err", err)
		}
		return state{Source: SourceUnset}, nil
	}
	if st.Source == "" {
		st.Source = SourceUnset
	}
	return st, nil
}

// Save atomically writes the state. Empty state writes `{}`, which Load
// round-trips to SourceUnset.
func (s *store) Save(st state) error {
	if s == nil || s.fs == nil {
		return nil
	}
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return s.fs.SaveFile(stateFilePath, string(body))
}

// Clear writes the inactive state.
func (s *store) Clear() error { return s.Save(state{}) }

// isMissingErr recognises os.IsNotExist and io/fs.ErrNotExist.
func isMissingErr(err error) bool {
	if err == nil {
		return false
	}
	return os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist)
}
