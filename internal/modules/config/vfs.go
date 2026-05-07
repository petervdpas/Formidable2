package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// vfs.go owns the materialised view of the on-disk Formidable layout
// under <context_folder>. Mirrors `Formidable/controls/configManager.js`
// buildVirtualStructure + getContextPath / getContextTemplatesPath /
// getContextStoragePath / getTemplateStorageInfo.
//
// Reads are TTL-cached (default 2s) so per-render rescans don't ladder
// into a thousand listdir calls. The TTL window is short enough that an
// external edit (gigot pull, manual file drop) becomes visible within a
// blink, and DirtyVirtualStructure() forces an immediate rebuild.

// GetVirtualStructure returns the cached VFS view, rebuilding if the
// cache is stale or empty. Auto-creates the templates and storage
// directories under the active context_folder on first build.
func (m *Manager) GetVirtualStructure() (*VirtualStructure, error) {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	cached := m.virtualStructure
	built := m.virtualStructureBuilt
	ttl := m.ttl
	now := m.nowFn
	m.mu.RUnlock()

	if cached != nil && now().Sub(built) < ttl {
		return cached, nil
	}

	vfs, err := m.buildVirtualStructure(cfg)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.virtualStructure = vfs
	m.virtualStructureBuilt = m.nowFn()
	m.mu.Unlock()
	return vfs, nil
}

// buildVirtualStructure walks the active context's templates folder and
// surfaces, for each <name>.yaml, a TemplateStorageFolder with the
// .meta.json files and image files under storage/<name>. The companion
// storage folders (and their images/ subfolder) are auto-created so
// later writes don't have to special-case the first-touch path.
func (m *Manager) buildVirtualStructure(cfg *Config) (*VirtualStructure, error) {
	context := cfg.ContextFolder
	if context == "" {
		context = "./"
	}
	base := m.fs.ResolvePath(context)
	templatesPath := filepath.Join(base, templatesDirName)
	storagePath := filepath.Join(base, storageDirName)

	if err := m.fs.EnsureDirectory(templatesPath); err != nil {
		return nil, fmt.Errorf("ensure templates dir: %w", err)
	}
	if err := m.fs.EnsureDirectory(storagePath); err != nil {
		return nil, fmt.Errorf("ensure storage dir: %w", err)
	}

	templateFiles, err := m.fs.ListFiles(templatesPath)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	folders := map[string]TemplateStorageFolder{}
	for _, file := range templateFiles {
		if !strings.HasSuffix(strings.ToLower(file), templateExt) {
			continue
		}
		name := strings.TrimSuffix(file, templateExt)
		tplStoragePath := filepath.Join(storagePath, name)
		imagesPath := filepath.Join(tplStoragePath, imagesDirName)

		if err := m.fs.EnsureDirectory(tplStoragePath); err != nil {
			return nil, fmt.Errorf("ensure storage/%s: %w", name, err)
		}
		if err := m.fs.EnsureDirectory(imagesPath); err != nil {
			return nil, fmt.Errorf("ensure storage/%s/images: %w", name, err)
		}

		metaFiles := []string{}
		entries, err := m.fs.ListFiles(tplStoragePath)
		if err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e, formExt) {
					metaFiles = append(metaFiles, e)
				}
			}
			sort.Strings(metaFiles)
		}

		imageFiles := []string{}
		entries, err = m.fs.ListFiles(imagesPath)
		if err == nil {
			imageFiles = append(imageFiles, entries...)
			sort.Strings(imageFiles)
		}

		folders[name] = TemplateStorageFolder{
			Name:       name,
			Filename:   file,
			Path:       tplStoragePath,
			MetaFiles:  metaFiles,
			ImageFiles: imageFiles,
		}
	}

	return &VirtualStructure{
		Context:                base,
		Templates:              templatesPath,
		Storage:                storagePath,
		TemplateStorageFolders: folders,
	}, nil
}

// GetContextPath returns the absolute path of the active context folder,
// auto-creating it if it doesn't exist yet.
func (m *Manager) GetContextPath() (string, error) {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return "", err
	}
	folder := cfg.ContextFolder
	if folder == "" {
		folder = "./"
	}
	abs := m.fs.ResolvePath(folder)
	if err := m.fs.EnsureDirectory(abs); err != nil {
		return "", fmt.Errorf("ensure context dir: %w", err)
	}
	return abs, nil
}

// GetContextTemplatesPath returns the absolute path of the templates
// folder under the active context.
func (m *Manager) GetContextTemplatesPath() (string, error) {
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return "", err
	}
	return vfs.Templates, nil
}

// GetContextStoragePath returns the absolute path of the storage folder
// under the active context.
func (m *Manager) GetContextStoragePath() (string, error) {
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return "", err
	}
	return vfs.Storage, nil
}

// GetTemplateStorageInfo returns the per-template storage record for
// templateFilename (e.g. "basic.yaml"), or nil if the template isn't
// registered in the current VFS view.
func (m *Manager) GetTemplateStorageInfo(templateFilename string) *TemplateStorageFolder {
	if templateFilename == "" {
		return nil
	}
	name := strings.TrimSuffix(templateFilename, templateExt)
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return nil
	}
	if info, ok := vfs.TemplateStorageFolders[name]; ok {
		out := info
		return &out
	}
	return nil
}
