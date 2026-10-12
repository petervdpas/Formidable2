package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// clientsDir is where a plugin ships api-clients, mirroring <AppRoot>/api-clients: <id>.yaml plus specs/<spec_file>.
const clientsDir = "api-clients"

// ClientInstaller is the api-client store as a plugin sees it. InstallClient is create-only; read serves files relative to the plugin's api-clients folder.
// A plugin ships <id>.yaml plus the document it names, or only specs/<id>.json (.yaml, .yml), from which the store derives the definition and reports derived = true.
// No credential travels with a plugin: an installed client has none until the user supplies it.
type ClientInstaller interface {
	HasClient(id string) bool
	InstallClient(id string, read func(name string) ([]byte, error)) (derived bool, err error)
	// CreateClient registers a client a script assembled: def is the JSON-shaped definition, spec the document. created is false when the id already exists, which is left untouched.
	CreateClient(def map[string]any, spec []byte) (created bool, err error)
}

// ClientAdmin is formidable.client.exists / create, bound to the running plugin.
type ClientAdmin interface {
	Exists(id string) bool
	Create(id string, opts map[string]any) (created bool, err error)
}

// Client* is the closed vocabulary of ClientInstall.Status; Vue maps each to a message through a literal table.
const (
	ClientInstalled = "installed"
	ClientExists    = "exists"
	ClientFailed    = "failed"
)

// ClientInstall is the outcome for one api-client a plugin declares. Error is set only on ClientFailed.
// Derived marks an installed client whose definition was drafted from the document alone, so the user knows to review more than the credential.
type ClientInstall struct {
	Plugin  string `json:"plugin"`
	ID      string `json:"id"`
	Status  string `json:"status"`
	Derived bool   `json:"derived,omitempty"`
	Error   string `json:"error,omitempty"`
}

// bundleReader serves a plugin's shipped api-clients folder, refusing any name that leaves it.
func bundleReader(pluginDir string) func(name string) ([]byte, error) {
	root := filepath.Join(pluginDir, clientsDir)
	return func(name string) ([]byte, error) {
		local := filepath.FromSlash(name)
		if !filepath.IsLocal(local) {
			return nil, fmt.Errorf("plugin: %q is outside the shipped api-clients folder", name)
		}
		return os.ReadFile(filepath.Join(root, local))
	}
}

// shipsClient reports whether the plugin folder carries the client: a definition, or a document named after it.
// A declared client the plugin does not ship is one its script creates (formidable.client.create) or the user made by hand, so Refresh does not install it and Run does not wait for it.
func shipsClient(pluginDir, id string) bool {
	root := filepath.Join(pluginDir, clientsDir)
	for _, name := range []string{
		id + ".yaml",
		filepath.Join("specs", id+".json"),
		filepath.Join("specs", id+".yaml"),
		filepath.Join("specs", id+".yml"),
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// installClients registers the shipped api-clients that do not exist yet. An existing client is skipped silently, so a Refresh never re-reports it.
func (m *Manager) installClients(p Plugin) []ClientInstall {
	if m.deps.Clients == nil {
		return nil
	}
	var out []ClientInstall
	for _, id := range p.Manifest.APIClients {
		if m.deps.Clients.HasClient(id) || !shipsClient(p.Dir, id) {
			continue
		}
		res := ClientInstall{Plugin: p.Manifest.ID, ID: id, Status: ClientInstalled}
		derived, err := m.deps.Clients.InstallClient(id, bundleReader(p.Dir))
		res.Derived = derived
		if err != nil {
			res.Status, res.Derived, res.Error = ClientFailed, false, err.Error()
			m.log.Warn("plugin: api-client install failed", "plugin", p.Manifest.ID, "client", id, "err", err)
		}
		out = append(out, res)
	}
	return out
}

// queueClientNotices holds outcomes until the frontend collects them; a repeat for the same plugin and client replaces the earlier one.
func (m *Manager) queueClientNotices(outcomes []ClientInstall) {
	if len(outcomes) == 0 {
		return
	}
	m.noticeMu.Lock()
	defer m.noticeMu.Unlock()
	if m.notices == nil {
		m.notices = map[[2]string]ClientInstall{}
	}
	for _, o := range outcomes {
		m.notices[[2]string{o.Plugin, o.ID}] = o
	}
}

// TakeClientNotices returns and clears the pending install outcomes, sorted by plugin then client.
// Installs happen on Refresh, which also runs at boot before any window exists, so the outcomes wait here to be shown.
func (m *Manager) TakeClientNotices() []ClientInstall {
	m.noticeMu.Lock()
	defer m.noticeMu.Unlock()
	out := make([]ClientInstall, 0, len(m.notices))
	for _, n := range m.notices {
		out = append(out, n)
	}
	m.notices = nil
	sort.Slice(out, func(i, j int) bool {
		if out[i].Plugin != out[j].Plugin {
			return out[i].Plugin < out[j].Plugin
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// missingClient returns the first shipped api-client that is not installed, or "".
func (m *Manager) missingClient(p Plugin) string {
	if m.deps.Clients == nil {
		return ""
	}
	for _, id := range p.Manifest.APIClients {
		if shipsClient(p.Dir, id) && !m.deps.Clients.HasClient(id) {
			return id
		}
	}
	return ""
}

// clientAdmin is ClientAdmin for one plugin's Run.
type clientAdmin struct {
	m *Manager
	p Plugin
}

func (a clientAdmin) Exists(id string) bool {
	return a.m.deps.Clients != nil && a.m.deps.Clients.HasClient(id)
}

// Create reads the document opts.spec names (a path inside the plugin folder), hands the rest of opts over as the definition, and queues the notice that tells the user to supply credentials.
func (a clientAdmin) Create(id string, opts map[string]any) (bool, error) {
	if a.m.deps.Clients == nil {
		return false, errors.New("not configured")
	}
	specPath, _ := opts["spec"].(string)
	if strings.TrimSpace(specPath) == "" {
		return false, errors.New(`option "spec" must name the OpenAPI document inside the plugin folder`)
	}
	local := filepath.FromSlash(specPath)
	if !filepath.IsLocal(local) {
		return false, fmt.Errorf("spec %q is outside the plugin folder", specPath)
	}
	spec, err := os.ReadFile(filepath.Join(a.p.Dir, local))
	if err != nil {
		return false, fmt.Errorf("cannot read spec %q: %w", specPath, err)
	}
	def := make(map[string]any, len(opts))
	for k, v := range opts {
		if k != "spec" {
			def[k] = v
		}
	}
	def["id"] = id
	created, err := a.m.deps.Clients.CreateClient(def, spec)
	if err != nil || !created {
		return false, err
	}
	a.m.queueClientNotices([]ClientInstall{{Plugin: a.p.Manifest.ID, ID: id, Status: ClientInstalled, Derived: true}})
	if a.m.deps.ClientNoticeOut != nil {
		a.m.deps.ClientNoticeOut()
	}
	return true, nil
}

// importedClients reports every api-client the imported plugin declares, in manifest order: what this import installed or failed on, and what was already there.
func (m *Manager) importedClients(p Plugin, outcomes []ClientInstall) []ClientInstall {
	if m.deps.Clients == nil {
		return nil
	}
	byID := map[string]ClientInstall{}
	for _, o := range outcomes {
		byID[o.ID] = o
	}
	var out []ClientInstall
	for _, id := range p.Manifest.APIClients {
		if o, ok := byID[id]; ok {
			out = append(out, o)
			continue
		}
		out = append(out, ClientInstall{Plugin: p.Manifest.ID, ID: id, Status: ClientExists})
	}
	return out
}
