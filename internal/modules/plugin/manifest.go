package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadManifest reads and validates <dir>/plugin.json, returning ErrManifestInvalid for shape problems and ErrManifestVersion for bad manifest_version.
// main.lua existence is checked here too: a manifest without a script is always a bug. Errors wrap with %w so errors.Is keeps working.
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

// FnNameFor returns the Lua global to call: the explicit Fn override, else the command ID.
func FnNameFor(c Command) string {
	if c.Fn != "" {
		return c.Fn
	}
	return c.ID
}

// validateManifest enforces the Manifest/Command field invariants, wrapping ErrManifestInvalid.
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
	// Empty run_mode behaves like "modal"; validating a set value catches typos at load time, not at runtime.
	switch m.RunMode {
	case "", RunModeModal, RunModeForm:
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

// validID enforces lowercase-ascii / digit / dash / underscore so an id can never escape the plugins root via path traversal.
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
