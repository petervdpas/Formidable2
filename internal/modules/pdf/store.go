package pdf

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"
)

// stateFilePath is the per-machine activation record relative to
// system.Manager's AppRoot. NOT in user.json — `browser_bin` is a
// machine-specific path and would break under gigot/git sync. The
// leading dot marks it as a hidden file alongside `.boot.json` (the
// other per-machine bootstrap record under config/). See
// design/pdf-export.md "Activation persistence".
const stateFilePath = "config/.pdf-state.json"

// state is the on-disk shape. ActivatedAt is omitzero so the
// inactive default is a clean `{}` rather than a misleading zero
// timestamp. Field tags are stable — do not rename without a
// migration.
//
// ExportDir is independent of activation: it survives Deactivate so
// the user's preferred export folder doesn't get wiped when they
// unbind a browser binary.
type state struct {
	BrowserBin  string    `json:"browser_bin,omitempty"`
	Source      Source    `json:"source,omitempty"`
	Version     string    `json:"version,omitempty"`
	ActivatedAt time.Time `json:"activated_at,omitzero"`
	ExportDir   string    `json:"export_dir,omitempty"`
}

// storeFS is the narrow filesystem surface the pdf module needs.
// *system.Manager satisfies it; tests pass an in-memory stub.
//
// ListDir is used by the cover library (Stage 6+): scanning
// <AppRoot>/pdf/covers/ to populate the cover-picker dropdown.
// Returns an empty slice for a missing directory so first-run boots
// (before scaffold) don't error.
type storeFS interface {
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	ListDir(path string) ([]string, error)
}

// store reads / writes pdf-state.json. All operations are tolerant:
// a missing file, malformed JSON, or a nil fs degrade to "inactive"
// rather than failing — losing the activation hint is preferable to
// blocking startup.
type store struct {
	fs  storeFS
	log *slog.Logger
}

// Load returns the persisted state, or the zero (inactive) value
// when no file exists / the file is malformed / the fs is nil. Real
// I/O errors (permission denied, etc.) bubble up so the caller can
// log them. Malformed JSON is reported via log.Warn but treated as
// "no state" so a corrupt state file doesn't brick the module.
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

// Save writes the state to disk. Atomic via system.Manager's
// SaveFile (temp+fsync+rename). Empty state writes `{}` — Load
// round-trips that to SourceUnset.
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

// Clear writes the inactive state. Equivalent to Save(state{}) but
// the intent is named — Deactivate calls Clear, not Save with a zero
// value.
func (s *store) Clear() error { return s.Save(state{}) }

// isMissingErr recognises os.IsNotExist and io/fs.ErrNotExist.
// system.Manager.LoadFile bubbles os.ReadFile errors which all match
// one of those.
func isMissingErr(err error) bool {
	if err == nil {
		return false
	}
	return os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist)
}
