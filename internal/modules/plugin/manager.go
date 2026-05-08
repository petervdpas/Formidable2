package plugin

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ManagerDeps groups the bridges plugins need at runtime. The
// access interfaces are optional; nil = "this namespace is
// disabled," and Lua calls into it surface a clear error.
//
// Logger may be nil (defaults to slog.Default).
//
// Editor is the fs surface used by Create/Save/Delete/GetSource —
// the workspace's CRUD methods. *system.Manager satisfies it; tests
// pass an in-test shim. When Editor is nil, CRUD methods return
// "editor fs not configured" instead of touching disk.
type ManagerDeps struct {
	PluginsDir string
	Logger     *slog.Logger

	KV         *KV
	Editor     editorFS
	Template   TemplateAccess
	Collection CollectionAccess
	Form       FormAccess
	Render     RenderAccess
	FS         FSAccess
	Exec       ExecRunner
}

// Manager owns the discovered plugin registry and runs commands.
// Concurrency: List/Get/Run hold a read lock; Refresh holds a
// write lock while replacing the map atomically. Plugin scripts
// themselves run with no Manager lock held — gopher-lua isn't
// goroutine-safe per LState, but each Run spawns a fresh state, so
// concurrent Runs are independent.
type Manager struct {
	deps ManagerDeps
	log  *slog.Logger

	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewManager constructs a Manager. Discovery doesn't run here —
// call Refresh after wiring (typically once at boot).
func NewManager(d ManagerDeps) *Manager {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Manager{deps: d, log: d.Logger, plugins: map[string]Plugin{}}
}

// Refresh re-scans <PluginsDir> and rebuilds the registry. Safe
// to call repeatedly. Best-effort: a corrupt manifest in one
// folder is logged and skipped — other plugins still load.
//
// Skipping rules:
//   - non-directories (stray README.md, archives, etc.)
//   - hidden folders (".kv", ".cache", anything starting with ".")
//   - folders missing plugin.json
//   - folders whose plugin.json fails validation
func (m *Manager) Refresh() error {
	if _, err := os.Stat(m.deps.PluginsDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No plugins folder yet → empty registry, not an error.
			m.mu.Lock()
			m.plugins = map[string]Plugin{}
			m.mu.Unlock()
			return nil
		}
		return fmt.Errorf("plugin: stat %s: %w", m.deps.PluginsDir, err)
	}

	entries, err := os.ReadDir(m.deps.PluginsDir)
	if err != nil {
		return fmt.Errorf("plugin: readdir %s: %w", m.deps.PluginsDir, err)
	}

	next := map[string]Plugin{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(m.deps.PluginsDir, e.Name())
		manifestPath := filepath.Join(dir, "plugin.json")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}
		manifest, err := LoadManifest(dir)
		if err != nil {
			m.log.Warn("plugin: skip invalid manifest",
				"dir", dir, "err", err)
			continue
		}
		// The on-disk folder name and the manifest id should agree
		// — otherwise two plugins with the same id could collide.
		if manifest.ID != e.Name() {
			m.log.Warn("plugin: id/folder mismatch — skipping",
				"folder", e.Name(), "manifest_id", manifest.ID)
			continue
		}
		next[manifest.ID] = Plugin{Manifest: manifest, Dir: dir}
	}

	m.mu.Lock()
	m.plugins = next
	m.mu.Unlock()
	return nil
}

// List returns the registered plugins, sorted by id for stable
// UI ordering.
func (m *Manager) List() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.ID < out[j].Manifest.ID
	})
	return out
}

// Get returns the plugin by id or (zero, false).
func (m *Manager) Get(id string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[id]
	return p, ok
}

// Run executes a command from a discovered plugin. ctx is the
// optional argument table passed to the Lua function (nil =
// no argument). Returns the function's converted return value
// plus any log lines emitted via formidable.log.* during the
// call.
func (m *Manager) Run(pluginID, commandID string, ctx map[string]any) (RunResult, error) {
	p, ok := m.Get(pluginID)
	if !ok {
		return RunResult{}, fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	cmd := findCommand(p.Manifest.Commands, commandID)
	if cmd == nil {
		return RunResult{}, fmt.Errorf("%w: %s.%s", ErrCommandNotFound, pluginID, commandID)
	}
	src, err := os.ReadFile(filepath.Join(p.Dir, "main.lua"))
	if err != nil {
		return RunResult{}, fmt.Errorf("plugin: read main.lua: %w", err)
	}

	var arg any
	if ctx != nil {
		arg = ctx
	}
	return runScript(scriptOpts{
		Source:     string(src),
		Fn:         FnNameFor(*cmd),
		Arg:        arg,
		PluginID:   pluginID,
		KV:         m.deps.KV,
		Template:   m.deps.Template,
		Collection: m.deps.Collection,
		Form:       m.deps.Form,
		Render:     m.deps.Render,
		FS:         m.deps.FS,
		Exec:       m.deps.Exec,
	})
}

func findCommand(cmds []Command, id string) *Command {
	for i := range cmds {
		if cmds[i].ID == id {
			return &cmds[i]
		}
	}
	return nil
}
