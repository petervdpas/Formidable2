package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// ManagerDeps groups the bridges plugins need at runtime. Access interfaces are optional: nil disables that namespace.
type ManagerDeps struct {
	PluginsDir string
	Logger     *slog.Logger

	KV            *KV
	Editor        editorFS
	Template      TemplateAccess
	Collection    CollectionAccess
	Form          FormAccess
	Render        RenderAccess
	FM            FMAccess
	FS            FSAccess
	Storage       StorageAccess
	Exec          ExecRunner
	API           HTTPClient
	Stats         StatsAccess
	Facets        FacetStatsAccess
	StatObject    StatObjectAccess
	RunBarOut     RunBarEmitter
	RunStatOut    RunStatusEmitter
	RunChartOut   RunChartEmitter
	RunOptionsOut RunOptionsEmitter
	// Locale supplies the active locale id at Run time; nil falls back to "en".
	Locale LocaleProvider
}

// LocaleProvider returns the active locale id, re-read per Run so a mid-session switch lands on the next Run.
type LocaleProvider interface {
	ActiveLocale() string
}

// Manager owns the discovered plugin registry and runs commands.
// Concurrency: List/Get/Run read-lock, Refresh write-locks while swapping the map. Scripts run with no Manager lock;
// each Run spawns a fresh LState (gopher-lua isn't goroutine-safe per state), so concurrent Runs are independent.
type Manager struct {
	deps ManagerDeps
	log  *slog.Logger

	mu      sync.RWMutex
	plugins map[string]Plugin

	// runActive is the one-at-a-time guard for Run; a second Run fails fast with ErrPluginBusy.
	runActive atomic.Bool

	// cancelMu protects cancelFn (set on Run start, called by Cancel, nilled on Run exit); never held around Run itself.
	cancelMu sync.Mutex
	cancelFn context.CancelFunc
}

// NewManager constructs a Manager; discovery runs on the first Refresh, not here.
func NewManager(d ManagerDeps) *Manager {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Manager{deps: d, log: d.Logger, plugins: map[string]Plugin{}}
}

// Refresh re-scans <PluginsDir> and rebuilds the registry, best-effort: a corrupt manifest is logged and skipped.
// Skips non-directories, hidden folders, folders missing plugin.json, and folders whose plugin.json fails validation.
func (m *Manager) Refresh() error {
	if _, err := os.Stat(m.deps.PluginsDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No plugins folder yet: empty registry, not an error.
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
		// Folder name and manifest id must agree, else two plugins could collide on id.
		if manifest.ID != e.Name() {
			m.log.Warn("plugin: id/folder mismatch - skipping",
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

// List returns the registered plugins sorted by id.
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

// ListForWorkspace returns the non-template-scoped plugins for ws (template-scoped ones surface only via ListForTemplate).
func (m *Manager) ListForWorkspace(ws string) []Plugin {
	return m.ListForTemplate(ws, "")
}

// ListForTemplate returns plugins for workspace ws given the active template: workspace plugins (empty Templates) always,
// plus template-scoped ones when template != "" and their Templates contains it. Empty template yields workspace-channel only.
func (m *Manager) ListForTemplate(ws, template string) []Plugin {
	if !isValidWorkspace(ws) {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Plugin
	for _, p := range m.plugins {
		if !slices.Contains(p.Manifest.Workspaces, ws) {
			continue
		}
		if len(p.Manifest.Templates) == 0 {
			out = append(out, p)
			continue
		}
		if template != "" && slices.Contains(p.Manifest.Templates, template) {
			out = append(out, p)
		}
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

// SaveFormValues writes each entry as its own KV slot keyed by field id, readable from Lua via formidable.kv.get(fieldKey).
// Keys outside `values` are preserved; no-op when KV is unavailable.
func (m *Manager) SaveFormValues(pluginID string, values map[string]any) error {
	if m.deps.KV == nil {
		return nil
	}
	for k, v := range values {
		if err := m.deps.KV.Set(pluginID, k, v); err != nil {
			return err
		}
	}
	return nil
}

// LoadFormValues returns the stored values for fieldKeys; missing keys are absent so the caller can fall back to field defaults.
func (m *Manager) LoadFormValues(pluginID string, fieldKeys []string) map[string]any {
	out := map[string]any{}
	if m.deps.KV == nil {
		return out
	}
	for _, k := range fieldKeys {
		v, ok, err := m.deps.KV.Get(pluginID, k)
		if err != nil || !ok {
			continue
		}
		out[k] = v
	}
	return out
}

// Run executes a plugin command; ctx is the optional Lua argument table. Returns the converted value plus formidable.log.* lines.
// Manager.Cancel() may fire from any goroutine to abort the VM; cancelled runs return ErrPluginCancelled.
func (m *Manager) Run(pluginID, commandID string, ctx map[string]any) (RunResult, error) {
	// CAS + cancelFn mutate under one lock so Cancel() never observes runActive=true with cancelFn=nil.
	m.cancelMu.Lock()
	if !m.runActive.CompareAndSwap(false, true) {
		m.cancelMu.Unlock()
		return RunResult{}, ErrPluginBusy
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel
	m.cancelMu.Unlock()
	defer func() {
		m.cancelMu.Lock()
		m.cancelFn = nil
		m.cancelMu.Unlock()
		cancel()
		m.runActive.Store(false)
	}()
	p, ok := m.Get(pluginID)
	if !ok {
		return RunResult{}, fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	cmd := findCommand(p.Manifest.Commands, commandID)
	if cmd == nil {
		return RunResult{}, fmt.Errorf("%w: %s.%s", ErrCommandNotFound, pluginID, commandID)
	}
	// Preflight before any Lua loads, so the Run modal shows a clean error instead of dying on the first api.fetch.
	if p.Manifest.RequiresInternalServer {
		if m.deps.API == nil || !m.deps.API.IsAvailable() {
			return RunResult{}, ErrServerNotRunning
		}
	}
	src, err := os.ReadFile(filepath.Join(p.Dir, "main.lua"))
	if err != nil {
		return RunResult{}, fmt.Errorf("plugin: read main.lua: %w", err)
	}

	var arg any
	if ctx != nil {
		arg = ctx
	}
	mode := p.Manifest.RunMode
	if mode == "" {
		mode = RunModeModal
	}
	return runScript(scriptOpts{
		Source: string(src),
		Fn:     FnNameFor(*cmd),
		Arg:    arg,
		Ctx:    runCtx,
		Plugin: PluginInfo{
			ID:                     p.Manifest.ID,
			Name:                   p.Manifest.Name,
			Version:                p.Manifest.Version,
			Author:                 p.Manifest.Author,
			Description:            p.Manifest.Description,
			Mode:                   mode,
			Command:                cmd.ID,
			RequiresInternalServer: p.Manifest.RequiresInternalServer,
			Debug:                  p.Manifest.Debug,
			Form:                   loadFormFields(p.Dir),
		},
		PluginID:      pluginID,
		KV:            m.deps.KV,
		Template:      m.deps.Template,
		Collection:    m.deps.Collection,
		Form:          m.deps.Form,
		Render:        m.deps.Render,
		FM:            m.deps.FM,
		FS:            m.deps.FS,
		Storage:       m.deps.Storage,
		Exec:          m.deps.Exec,
		API:           m.deps.API,
		Stats:         m.deps.Stats,
		Facets:        m.deps.Facets,
		StatObject:    m.deps.StatObject,
		RunBarOut:     m.deps.RunBarOut,
		RunStatOut:    m.deps.RunStatOut,
		RunChartOut:   m.deps.RunChartOut,
		RunOptionsOut: m.deps.RunOptionsOut,
		I18nMessages:  m.messagesForPlugin(pluginID),
	})
}

// messagesForPlugin returns this plugin's active-locale translations with the `plugin.<id>.` prefix stripped, or nil when none.
func (m *Manager) messagesForPlugin(pluginID string) map[string]string {
	locale := "en"
	if m.deps.Locale != nil {
		if active := m.deps.Locale.ActiveLocale(); active != "" {
			locale = active
		}
	}
	all := m.MessagesForLocale(locale)
	if len(all) == 0 {
		return nil
	}
	prefix := "plugin." + pluginID + "."
	out := map[string]string{}
	for k, v := range all {
		if after, ok := strings.CutPrefix(k, prefix); ok {
			out[after] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Cancel aborts the running plugin (if any): the VM's bound context fires Done() and gopher-lua stops at the next instruction.
func (m *Manager) Cancel() {
	m.cancelMu.Lock()
	fn := m.cancelFn
	m.cancelMu.Unlock()
	if fn != nil {
		fn()
	}
}

// loadFormFields reads <pluginDir>/form.json, tolerating a bare array (canonical) or a {fields:[...]} object (legacy); nil on any failure.
func loadFormFields(pluginDir string) []map[string]any {
	raw, err := os.ReadFile(filepath.Join(pluginDir, "form.json"))
	if err != nil {
		return nil
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var obj struct {
		Fields []map[string]any `json:"fields"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Fields
	}
	return nil
}

func findCommand(cmds []Command, id string) *Command {
	for i := range cmds {
		if cmds[i].ID == id {
			return &cmds[i]
		}
	}
	return nil
}
