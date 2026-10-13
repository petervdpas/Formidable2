package plugin

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// shippedTemplatesDir is where a plugin holds the templates it brings along: <plugin>/templates/<name>.yaml.
// The files themselves are the list; the manifest's `templates` key means something else (which templates the plugin is shown for).
const shippedTemplatesDir = "templates"

// ErrTemplateNotShipped: the plugin holds no template under that file name.
var ErrTemplateNotShipped = errors.New("plugin: the plugin holds no such template")

// ErrTemplateInvalid: the YAML handed over is not a template the template store accepts.
var ErrTemplateInvalid = errors.New("plugin: not a usable template")

// TemplateInstaller is the template store as a plugin sees it. CreateTemplate is create-only: created is false when a template of that name exists, which is left untouched.
// InspectTemplate parses and validates without storing; TemplateSource returns an existing template's YAML so it can be copied into a plugin.
type TemplateInstaller interface {
	HasTemplate(name string) bool
	CreateTemplate(name string, yaml []byte) (created bool, err error)
	InspectTemplate(name string, yaml []byte) (TemplateInfo, error)
	TemplateSource(name string) (string, error)
	// TemplateFiles lists the template file names in the open context folder.
	TemplateFiles() ([]string, error)
	// ReplaceTemplate overwrites an existing template; TemplateDiffers says whether the existing one differs from yaml in more than its author.
	ReplaceTemplate(name string, yaml []byte) error
	TemplateDiffers(name string, yaml []byte) (bool, error)
}

// ErrTemplateNotCreated: a template the plugin holds could not be created when a Run started.
var ErrTemplateNotCreated = errors.New("plugin: a template this plugin brings could not be created")

// BindableTemplate is one template a plugin can be bound to (Manifest.Templates): every template of the context folder, plus the ones the plugin holds.
// A held template is offered before it exists, so a plugin can be bound to the template it brings.
type BindableTemplate struct {
	File   string `json:"file"`
	Held   bool   `json:"held"`
	Exists bool   `json:"exists"`
}

// ListBindableTemplates returns the binding choices for a plugin, sorted by file name.
func (m *Manager) ListBindableTemplates(pluginID string) ([]BindableTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	existing, err := m.deps.Templates.TemplateFiles()
	if err != nil {
		return nil, err
	}
	held, err := m.shippedTemplateFiles(p)
	if err != nil {
		return nil, err
	}
	byFile := map[string]*BindableTemplate{}
	for _, f := range existing {
		byFile[f] = &BindableTemplate{File: f, Exists: true}
	}
	for _, f := range held {
		if row, ok := byFile[f]; ok {
			row.Held = true
		} else {
			byFile[f] = &BindableTemplate{File: f, Held: true}
		}
	}
	out := make([]BindableTemplate, 0, len(byFile))
	for _, row := range byFile {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// createHeldTemplates makes the templates the plugin holds before its script starts, create-only and announced.
// A plugin bound to its own template cannot be started from that template until it exists, so this happens at the Run the user asked for, not in the script.
func (m *Manager) createHeldTemplates(p Plugin) error {
	var created []ClientInstall
	for _, res := range m.installTemplates(p) {
		switch res.Status {
		case ClientFailed:
			return fmt.Errorf("%w: %s: %s", ErrTemplateNotCreated, res.File, res.Error)
		case ClientInstalled:
			created = append(created, ClientInstall{Kind: NoticeTemplate, Plugin: p.Manifest.ID, ID: res.File, Status: ClientInstalled})
		}
	}
	if len(created) > 0 {
		m.queueClientNotices(created)
		if m.deps.ClientNoticeOut != nil {
			m.deps.ClientNoticeOut()
		}
	}
	return nil
}

// TemplateInfo is what a template's YAML says about itself.
type TemplateInfo struct {
	Name   string
	Fields int
}

// TemplateAdmin is formidable.template.install, bound to the running plugin.
type TemplateAdmin interface {
	Install(file string) (created bool, err error)
}

// ShippedTemplate is one template a plugin holds, as the editor's Templates tab shows it.
// Installed says whether a template of that file name exists in the open context folder; Differs, whether that one differs from the plugin's copy.
type ShippedTemplate struct {
	File      string `json:"file"`
	Name      string `json:"name,omitempty"`
	Fields    int    `json:"fields"`
	Installed bool   `json:"installed"`
	Differs   bool   `json:"differs"`
	Error     string `json:"error,omitempty"`
}

// TemplateInstall is the outcome for one template on a zip import; Status uses the Client* vocabulary.
// Differs is set on ClientExists when the template that is there differs from the plugin's copy, so the user knows a replace is on offer.
type TemplateInstall struct {
	Plugin  string `json:"plugin"`
	File    string `json:"file"`
	Status  string `json:"status"`
	Differs bool   `json:"differs,omitempty"`
	Error   string `json:"error,omitempty"`
}

// validTemplateFile accepts a plain "<name>.yaml", so the name cannot leave the plugin's templates folder; the template store applies its own rule on top.
func validTemplateFile(file string) bool {
	stem := strings.TrimSuffix(file, ".yaml")
	return stem != "" && stem != file && !strings.HasPrefix(file, ".") &&
		!strings.ContainsAny(file, `/\`) && !strings.Contains(file, "..") && strings.TrimSpace(file) == file
}

func (m *Manager) shippedTemplatePath(p Plugin, file string) string {
	return filepath.Join(p.Dir, shippedTemplatesDir, file)
}

// shippedTemplateFiles lists the template files a plugin holds, sorted.
func (m *Manager) shippedTemplateFiles(p Plugin) ([]string, error) {
	entries, err := m.deps.Editor.ListDir(filepath.Join(p.Dir, shippedTemplatesDir))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if validTemplateFile(e) {
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out, nil
}

// templatePlugin resolves the plugin and checks the two things every templates call needs: an editor fs and a template store.
func (m *Manager) templatePlugin(pluginID string) (Plugin, error) {
	p, err := m.editablePlugin(pluginID)
	if err != nil {
		return Plugin{}, err
	}
	if m.deps.Templates == nil {
		return Plugin{}, errors.New("plugin: template store not configured")
	}
	return p, nil
}

// heldTemplate reads the YAML of a template the plugin holds.
func (m *Manager) heldTemplate(p Plugin, file string) (string, error) {
	if !validTemplateFile(file) {
		return "", fmt.Errorf("%w: bad template file name %q", ErrManifestInvalid, file)
	}
	path := m.shippedTemplatePath(p, file)
	if !m.deps.Editor.FileExists(path) {
		return "", fmt.Errorf("%w: %s", ErrTemplateNotShipped, file)
	}
	return m.deps.Editor.LoadFile(path)
}

// ListShippedTemplates describes every template the plugin holds.
func (m *Manager) ListShippedTemplates(pluginID string) ([]ShippedTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	files, err := m.shippedTemplateFiles(p)
	if err != nil {
		return nil, err
	}
	out := make([]ShippedTemplate, 0, len(files))
	for _, file := range files {
		row := ShippedTemplate{File: file, Installed: m.deps.Templates.HasTemplate(file)}
		raw, err := m.deps.Editor.LoadFile(m.shippedTemplatePath(p, file))
		if err == nil {
			var info TemplateInfo
			if info, err = m.deps.Templates.InspectTemplate(file, []byte(raw)); err == nil {
				row.Name, row.Fields = info.Name, info.Fields
				if row.Installed {
					row.Differs, err = m.deps.Templates.TemplateDiffers(file, []byte(raw))
				}
			}
		}
		if err != nil {
			row.Error = err.Error()
		}
		out = append(out, row)
	}
	return out, nil
}

// GetShippedTemplate returns the YAML of one template the plugin holds.
func (m *Manager) GetShippedTemplate(pluginID, file string) (string, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return "", err
	}
	return m.heldTemplate(p, file)
}

// SaveShippedTemplate makes the plugin hold yaml as templates/<file>. The YAML is validated first and refused if the template store would not accept it.
// Saving creates no template in the context folder: that is CreateShippedTemplate, which the user triggers.
func (m *Manager) SaveShippedTemplate(pluginID, file, yaml string) ([]ShippedTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validTemplateFile(file) {
		return nil, fmt.Errorf("%w: bad template file name %q", ErrManifestInvalid, file)
	}
	if _, err := m.deps.Templates.InspectTemplate(file, []byte(yaml)); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateInvalid, err)
	}
	if err := m.deps.Editor.EnsureDirectory(filepath.Join(p.Dir, shippedTemplatesDir)); err != nil {
		return nil, err
	}
	if err := m.deps.Editor.SaveFile(m.shippedTemplatePath(p, file), yaml); err != nil {
		return nil, err
	}
	return m.ListShippedTemplates(pluginID)
}

// AddShippedTemplateFrom copies a template that exists in the context folder into the plugin, so a template built in the Templates workspace can travel with the plugin.
// A file the plugin already holds under that name is replaced: this is the user choosing to take the current version along.
func (m *Manager) AddShippedTemplateFrom(pluginID, file string) ([]ShippedTemplate, error) {
	if _, err := m.templatePlugin(pluginID); err != nil {
		return nil, err
	}
	if !validTemplateFile(file) {
		return nil, fmt.Errorf("%w: bad template file name %q", ErrManifestInvalid, file)
	}
	src, err := m.deps.Templates.TemplateSource(file)
	if err != nil {
		return nil, err
	}
	return m.SaveShippedTemplate(pluginID, file, src)
}

// CreateShippedTemplate creates the template in the context folder from the copy the plugin holds, on the user's request.
// A template that already exists there is the user's and is left untouched; the rows report it as installed either way.
func (m *Manager) CreateShippedTemplate(pluginID, file string) ([]ShippedTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	raw, err := m.heldTemplate(p, file)
	if err != nil {
		return nil, err
	}
	if _, err := m.deps.Templates.CreateTemplate(file, []byte(raw)); err != nil {
		return nil, err
	}
	return m.ListShippedTemplates(pluginID)
}

// ReplaceShippedTemplate overwrites the template in the context folder with the copy the plugin holds. It exists for one case: the user asks for the plugin's improved version.
// Nothing else replaces a template, because doing so discards the user's own changes to it. Records stay; a field the new version drops keeps its stored values but is no longer shown.
func (m *Manager) ReplaceShippedTemplate(pluginID, file string) ([]ShippedTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	raw, err := m.heldTemplate(p, file)
	if err != nil {
		return nil, err
	}
	if err := m.deps.Templates.ReplaceTemplate(file, []byte(raw)); err != nil {
		return nil, err
	}
	return m.ListShippedTemplates(pluginID)
}

// RemoveShippedTemplate takes the template out of the plugin. A template created from it in the context folder stays, with its records.
func (m *Manager) RemoveShippedTemplate(pluginID, file string) ([]ShippedTemplate, error) {
	p, err := m.templatePlugin(pluginID)
	if err != nil {
		return nil, err
	}
	if !validTemplateFile(file) {
		return nil, fmt.Errorf("%w: bad template file name %q", ErrManifestInvalid, file)
	}
	if path := m.shippedTemplatePath(p, file); m.deps.Editor.FileExists(path) {
		if err := m.deps.Editor.DeleteFile(path); err != nil {
			return nil, err
		}
	}
	return m.ListShippedTemplates(pluginID)
}

// installTemplates creates the templates a plugin holds that do not exist yet, and reports each one. Called on zip import and at the start of a Run, never on Refresh.
func (m *Manager) installTemplates(p Plugin) []TemplateInstall {
	if m.deps.Templates == nil || m.deps.Editor == nil {
		return nil
	}
	files, err := m.shippedTemplateFiles(p)
	if err != nil {
		m.log.Warn("plugin: cannot list held templates", "plugin", p.Manifest.ID, "err", err)
		return nil
	}
	var out []TemplateInstall
	for _, file := range files {
		res := TemplateInstall{Plugin: p.Manifest.ID, File: file, Status: ClientExists}
		raw, err := m.deps.Editor.LoadFile(m.shippedTemplatePath(p, file))
		created := false
		if err == nil {
			created, err = m.deps.Templates.CreateTemplate(file, []byte(raw))
		}
		switch {
		case err != nil:
			res.Status, res.Error = ClientFailed, err.Error()
			m.log.Warn("plugin: template create failed", "plugin", p.Manifest.ID, "template", file, "err", err)
		case created:
			res.Status = ClientInstalled
		default:
			// Left alone, but say so when the plugin's copy is a different version.
			res.Differs, _ = m.deps.Templates.TemplateDiffers(file, []byte(raw))
		}
		out = append(out, res)
	}
	return out
}

// templateAdmin is TemplateAdmin for one plugin's Run.
type templateAdmin struct {
	m *Manager
	p Plugin
}

// Install creates the template from the copy the plugin holds, create-only, and queues the notice that tells the user a template was added.
func (a templateAdmin) Install(file string) (bool, error) {
	if a.m.deps.Templates == nil || a.m.deps.Editor == nil {
		return false, errors.New("not configured")
	}
	raw, err := a.m.heldTemplate(a.p, file)
	if err != nil {
		return false, err
	}
	created, err := a.m.deps.Templates.CreateTemplate(file, []byte(raw))
	if err != nil || !created {
		return false, err
	}
	a.m.queueClientNotices([]ClientInstall{{Kind: NoticeTemplate, Plugin: a.p.Manifest.ID, ID: file, Status: ClientInstalled}})
	if a.m.deps.ClientNoticeOut != nil {
		a.m.deps.ClientNoticeOut()
	}
	return true, nil
}
