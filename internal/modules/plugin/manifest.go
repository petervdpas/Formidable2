package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadManifest reads <dir>/plugin.json and validates it against
// the current schema. Returns ErrManifestInvalid for any shape
// problem (missing file, malformed JSON, missing required field)
// and ErrManifestVersion for unsupported manifest_version. The
// returned Manifest has its `Dir`-side validation done - main.lua
// existence is checked here too because shipping a manifest
// without a script is always a bug, not a "load lazily" concern.
//
// Wrap detail in fmt.Errorf("%w: ...") so errors.Is(...) keeps
// working but a logger gets the underlying cause.
func LoadManifest(dir string) (Manifest, error) {
	path := filepath.Join(dir, "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read %s: %v", ErrManifestInvalid, path, err)
	}

	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: parse %s: %v", ErrManifestInvalid, path, err)
	}

	if m.ManifestVersion != ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("%w: got %d, want %d",
			ErrManifestVersion, m.ManifestVersion, ManifestSchemaVersion)
	}

	if err := validateManifest(&m); err != nil {
		return Manifest{}, err
	}

	if _, err := os.Stat(filepath.Join(dir, "main.lua")); err != nil {
		return Manifest{}, fmt.Errorf("%w: missing main.lua in %s", ErrManifestInvalid, dir)
	}
	return m, nil
}

// FnNameFor returns the Lua global name to call for a command:
// the explicit Fn override when set, else the command ID itself.
func FnNameFor(c Command) string {
	if c.Fn != "" {
		return c.Fn
	}
	return c.ID
}

// validateManifest enforces the field-level invariants documented
// on Manifest/Command. Errors wrap ErrManifestInvalid so callers
// can branch with errors.Is.
func validateManifest(m *Manifest) error {
	if !validID(m.ID) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: empty name", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("%w: empty version", ErrManifestInvalid)
	}
	if len(m.Commands) == 0 {
		return fmt.Errorf("%w: at least one command required", ErrManifestInvalid)
	}
	// run_mode is optional - empty behaves like "modal" - but if set
	// it must name one of the known constants. Catches typos like
	// "Form" / "Modal" early so authors get a load-time error
	// rather than a silent fallback at runtime.
	switch m.RunMode {
	case "", RunModeModal, RunModeForm:
		// ok
	default:
		return fmt.Errorf("%w: bad run_mode %q (want %q or %q)",
			ErrManifestInvalid, m.RunMode, RunModeModal, RunModeForm)
	}
	seenWs := make(map[string]struct{}, len(m.Workspaces))
	for i, ws := range m.Workspaces {
		if !isValidWorkspace(ws) {
			return fmt.Errorf("%w: workspaces[%d] %q is not a known workspace id",
				ErrManifestInvalid, i, ws)
		}
		if _, dup := seenWs[ws]; dup {
			return fmt.Errorf("%w: workspaces lists %q twice",
				ErrManifestInvalid, ws)
		}
		seenWs[ws] = struct{}{}
	}
	seenTpl := make(map[string]struct{}, len(m.Templates))
	for i, tpl := range m.Templates {
		if strings.TrimSpace(tpl) == "" {
			return fmt.Errorf("%w: templates[%d] is empty", ErrManifestInvalid, i)
		}
		if _, dup := seenTpl[tpl]; dup {
			return fmt.Errorf("%w: templates lists %q twice", ErrManifestInvalid, tpl)
		}
		seenTpl[tpl] = struct{}{}
	}
	for i, c := range m.Commands {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("%w: command[%d] empty id", ErrManifestInvalid, i)
		}
	}
	return nil
}

// validID enforces the on-disk-folder-name contract: lowercase
// ascii letters, digits, dash, underscore. Rejects "/", "..", and
// empty so a hand-edited manifest can never escape the plugins
// root via path traversal when we use the id to resolve files.
func validID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
