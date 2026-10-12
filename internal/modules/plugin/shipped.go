package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// ErrClientDocumentInvalid: the text handed to SaveShippedClient is not an OpenAPI document the client store can read.
var ErrClientDocumentInvalid = errors.New("plugin: not a usable OpenAPI document")

// shippedSpecExts are the extensions a shipped document may carry, in lookup order.
var shippedSpecExts = []string{".json", ".yaml", ".yml"}

// ShippedClient is one api-client a plugin declares, as the editor's API-clients tab shows it.
// Document is the file under api-clients/specs the client is generated from ("" when the plugin ships none and a script or the user creates it).
// Name and BaseURL are what the plugin holds for the client; Title and Server are what the document itself declares, used when the held value is empty.
// HasDefinition marks a hand-written definition with bindings of its own, beyond settings.
type ShippedClient struct {
	ID            string `json:"id"`
	Document      string `json:"document,omitempty"`
	Name          string `json:"name,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	Server        string `json:"server,omitempty"`
	HasDefinition bool   `json:"has_definition"`
	Installed     bool   `json:"installed"`
	Title         string `json:"title,omitempty"`
	Operations    int    `json:"operations"`
	Error         string `json:"error,omitempty"`
}

func (m *Manager) shippedDir(p Plugin) string { return filepath.Join(p.Dir, clientsDir) }

// shippedDocument returns the document file name a plugin ships for id, or "".
func (m *Manager) shippedDocument(p Plugin, id string) string {
	for _, ext := range shippedSpecExts {
		if m.deps.Editor.FileExists(filepath.Join(m.shippedDir(p), "specs", id+ext)) {
			return id + ext
		}
	}
	return ""
}

// shippedDefinition returns the text of the definition the plugin ships for id, and whether there is one.
func (m *Manager) shippedDefinition(p Plugin, id string) (string, bool) {
	path := filepath.Join(m.shippedDir(p), id+".yaml")
	if !m.deps.Editor.FileExists(path) {
		return "", false
	}
	raw, err := m.deps.Editor.LoadFile(path)
	if err != nil {
		return "", false
	}
	return raw, true
}

// SetShippedSettings makes the plugin hold a display name and a base URL for clientID, so the client is created with them on every machine; an empty value clears that setting and the document's own title or server applies again.
// They are stored in the shipped definition api-clients/<clientID>.yaml, which is removed again when settings were all it held. Like the document, they only shape a client still to be created.
func (m *Manager) SetShippedSettings(pluginID, clientID, name, baseURL string) ([]ShippedClient, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validID(clientID) || !slices.Contains(p.Manifest.APIClients, clientID) {
		return nil, fmt.Errorf("%w: api-client %q is not declared by %s", ErrManifestInvalid, clientID, pluginID)
	}
	if m.deps.Clients == nil {
		return nil, errors.New("plugin: api-client store not configured")
	}
	def, _ := m.shippedDefinition(p, clientID)
	out, err := m.deps.Clients.WithSettings([]byte(def), clientID, ClientSettings{Name: name, BaseURL: baseURL})
	if err != nil {
		return nil, err
	}
	path := filepath.Join(m.shippedDir(p), clientID+".yaml")
	switch {
	case out != nil:
		if err := m.deps.Editor.EnsureDirectory(m.shippedDir(p)); err != nil {
			return nil, err
		}
		if err := m.deps.Editor.SaveFile(path, string(out)); err != nil {
			return nil, err
		}
	case m.deps.Editor.FileExists(path):
		if err := m.deps.Editor.DeleteFile(path); err != nil {
			return nil, err
		}
	}
	return m.ListShippedClients(pluginID)
}

func (m *Manager) editablePlugin(pluginID string) (Plugin, error) {
	if m.deps.Editor == nil {
		return Plugin{}, errors.New("plugin: editor fs not configured")
	}
	p, ok := m.Get(pluginID)
	if !ok {
		return Plugin{}, fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	return p, nil
}

// ListShippedClients describes every api-client the plugin declares, in manifest order.
func (m *Manager) ListShippedClients(pluginID string) ([]ShippedClient, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	out := make([]ShippedClient, 0, len(p.Manifest.APIClients))
	for _, id := range p.Manifest.APIClients {
		row := ShippedClient{
			ID:        id,
			Document:  m.shippedDocument(p, id),
			Installed: m.deps.Clients != nil && m.deps.Clients.HasClient(id),
		}
		if def, ok := m.shippedDefinition(p, id); ok && m.deps.Clients != nil {
			settings, err := m.deps.Clients.ReadSettings([]byte(def))
			if err != nil {
				row.Error = err.Error()
			}
			row.Name, row.BaseURL, row.HasDefinition = settings.Name, settings.BaseURL, settings.Custom
		}
		if row.Document != "" && m.deps.Clients != nil {
			raw, err := m.deps.Editor.LoadFile(filepath.Join(m.shippedDir(p), "specs", row.Document))
			if err == nil {
				var info DocumentInfo
				if info, err = m.deps.Clients.InspectDocument([]byte(raw)); err == nil {
					row.Title, row.Operations, row.Server = info.Title, info.Operations, info.Server
				}
			}
			if err != nil {
				row.Error = err.Error()
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// GetShippedDocument returns the text of the document the plugin ships for clientID, or "" when it ships none.
func (m *Manager) GetShippedDocument(pluginID, clientID string) (string, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return "", err
	}
	if !validID(clientID) {
		return "", fmt.Errorf("%w: bad api-client id %q", ErrManifestInvalid, clientID)
	}
	name := m.shippedDocument(p, clientID)
	if name == "" {
		return "", nil
	}
	return m.deps.Editor.LoadFile(filepath.Join(m.shippedDir(p), "specs", name))
}

// SaveShippedClient makes the plugin ship document as the source of api-client clientID: the text is parsed first and refused if unreadable,
// stored as api-clients/specs/<clientID>.<ext>, and the id is added to the manifest's api_clients.
// Saving creates no client: that is GenerateShippedClient, which the user triggers.
func (m *Manager) SaveShippedClient(pluginID, clientID, document string) ([]ShippedClient, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validID(clientID) {
		return nil, fmt.Errorf("%w: bad api-client id %q", ErrManifestInvalid, clientID)
	}
	if m.deps.Clients == nil {
		return nil, errors.New("plugin: api-client store not configured")
	}
	if strings.TrimSpace(document) == "" {
		return nil, fmt.Errorf("%w: empty", ErrClientDocumentInvalid)
	}
	info, err := m.deps.Clients.InspectDocument([]byte(document))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClientDocumentInvalid, err)
	}

	specs := filepath.Join(m.shippedDir(p), "specs")
	if err := m.deps.Editor.EnsureDirectory(specs); err != nil {
		return nil, err
	}
	if err := m.deps.Editor.SaveFile(filepath.Join(specs, clientID+info.Ext), document); err != nil {
		return nil, err
	}
	// One document per client: a copy under another extension would win or lose by lookup order, not by intent.
	for _, ext := range shippedSpecExts {
		if stale := filepath.Join(specs, clientID+ext); ext != info.Ext && m.deps.Editor.FileExists(stale) {
			if err := m.deps.Editor.DeleteFile(stale); err != nil {
				return nil, err
			}
		}
	}
	if !slices.Contains(p.Manifest.APIClients, clientID) {
		if err := m.writeAPIClients(p, append(slices.Clone(p.Manifest.APIClients), clientID)); err != nil {
			return nil, err
		}
	}
	return m.afterShippedChange(pluginID)
}

// ErrClientNotShipped: the plugin holds neither a document nor a definition for the client, so there is nothing to generate it from.
var ErrClientNotShipped = errors.New("plugin: no document for this api-client in the plugin")

// GenerateShippedClient creates the api-client from what the plugin ships, on the user's request, and returns the reason when it cannot be made.
// A client that already exists is left untouched and reported as such in the rows.
func (m *Manager) GenerateShippedClient(pluginID, clientID string) ([]ShippedClient, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validID(clientID) || !slices.Contains(p.Manifest.APIClients, clientID) {
		return nil, fmt.Errorf("%w: api-client %q is not declared by %s", ErrManifestInvalid, clientID, pluginID)
	}
	if m.deps.Clients == nil {
		return nil, errors.New("plugin: api-client store not configured")
	}
	if m.deps.Clients.HasClient(clientID) {
		return m.ListShippedClients(pluginID)
	}
	if !shipsClient(p.Dir, clientID) {
		return nil, fmt.Errorf("%w: %s", ErrClientNotShipped, clientID)
	}
	derived, err := m.deps.Clients.InstallClient(clientID, bundleReader(p.Dir))
	if err != nil {
		return nil, err
	}
	m.queueClientNotices([]ClientInstall{{Plugin: pluginID, ID: clientID, Status: ClientInstalled, Derived: derived}})
	if m.deps.ClientNoticeOut != nil {
		m.deps.ClientNoticeOut()
	}
	return m.ListShippedClients(pluginID)
}

// RemoveShippedClient stops the plugin declaring and shipping clientID: its document and definition leave the plugin folder and the id leaves api_clients.
// The installed client, if any, stays: it may hold a credential and other plugins or fields may use it.
func (m *Manager) RemoveShippedClient(pluginID, clientID string) ([]ShippedClient, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validID(clientID) {
		return nil, fmt.Errorf("%w: bad api-client id %q", ErrManifestInvalid, clientID)
	}
	files := []string{filepath.Join(m.shippedDir(p), clientID+".yaml")}
	for _, ext := range shippedSpecExts {
		files = append(files, filepath.Join(m.shippedDir(p), "specs", clientID+ext))
	}
	for _, f := range files {
		if m.deps.Editor.FileExists(f) {
			if err := m.deps.Editor.DeleteFile(f); err != nil {
				return nil, err
			}
		}
	}
	if slices.Contains(p.Manifest.APIClients, clientID) {
		kept := slices.DeleteFunc(slices.Clone(p.Manifest.APIClients), func(id string) bool { return id == clientID })
		if err := m.writeAPIClients(p, kept); err != nil {
			return nil, err
		}
	}
	return m.afterShippedChange(pluginID)
}

// writeAPIClients rewrites only api_clients in the plugin.json on disk, so nothing else the file holds is disturbed.
func (m *Manager) writeAPIClients(p Plugin, ids []string) error {
	onDisk, err := LoadManifest(p.Dir)
	if err != nil {
		return err
	}
	onDisk.APIClients = ids
	raw, err := SerializeManifest(onDisk)
	if err != nil {
		return err
	}
	return m.deps.Editor.SaveFile(filepath.Join(p.Dir, "plugin.json"), string(raw))
}

// afterShippedChange refreshes the registry so the manifest change is live, and returns the fresh rows.
func (m *Manager) afterShippedChange(pluginID string) ([]ShippedClient, error) {
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m.ListShippedClients(pluginID)
}
