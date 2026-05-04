package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// fs is the narrow filesystem surface this module needs. *system.Manager
// satisfies it. Declared here per the architecture rule that consumers
// own their dependency interfaces.
type fs interface {
	ResolvePath(segments ...string) string
	JoinPath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	CopyFile(from, to string, overwrite bool) error
	ListFiles(dir string) ([]string, error)
}

const (
	configDirName        = "config"
	bootFileName         = "boot.json"
	defaultProfileName   = "user.json"
	templatesDirName     = "templates"
	storageDirName       = "storage"
	imagesDirName        = "images"
	templateExt          = ".yaml"
	formExt              = ".meta.json"
	defaultVFSCacheTTL   = 2 * time.Second
)

// Manager owns config + VFS state. Methods are safe for concurrent use.
type Manager struct {
	fs      fs
	log     *slog.Logger
	journal JournalConfigurer

	mu                   sync.RWMutex
	configPath           string // absolute path to active profile JSON
	cached               *Config
	virtualStructure     *VirtualStructure
	virtualStructureBuilt time.Time
	ttl                  time.Duration
	nowFn                func() time.Time
}

// NewManager constructs and initializes the config manager. Initialization
// resolves the boot profile, ensures config dir + user.json exist, and
// loads the active profile into the cache.
func NewManager(filesystem fs, log *slog.Logger) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{
		fs:    filesystem,
		log:   log,
		ttl:   defaultVFSCacheTTL,
		nowFn: time.Now,
	}
	if err := m.initialize(); err != nil {
		return nil, fmt.Errorf("config init: %w", err)
	}
	return m, nil
}

// SetJournal wires the journal hook. Safe to call before or after init;
// nil disables journal sync. Re-applies current config to the journal.
func (m *Manager) SetJournal(j JournalConfigurer) {
	m.mu.Lock()
	m.journal = j
	cfg := m.cached
	m.mu.Unlock()
	if cfg != nil {
		m.syncJournal(cfg)
	}
}

// SetTTL overrides the virtual-structure cache TTL. Mainly for tests.
func (m *Manager) SetTTL(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttl = d
}

// SetNowFn injects a clock for tests.
func (m *Manager) SetNowFn(fn func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nowFn = fn
}

// initialize is called once by NewManager. It does not call out to the
// journal yet — that happens lazily on first Load.
func (m *Manager) initialize() error {
	if err := m.fs.EnsureDirectory(configDirName); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}
	profile, err := m.resolveBootProfile()
	if err != nil {
		return err
	}
	m.setConfigPath(profile)
	// Eagerly seed the active profile file so listing/export works even
	// before the first LoadUserConfig call.
	return m.ensureUserConfigFile()
}

// ─────────────────────────────────────────────────────────────────────
// Boot profile
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) bootPath() string {
	return m.fs.ResolvePath(configDirName, bootFileName)
}

// resolveBootProfile reads or creates config/boot.json, returns the
// active profile filename to use.
func (m *Manager) resolveBootProfile() (string, error) {
	bootPath := m.bootPath()
	if !m.fs.FileExists(bootPath) {
		if err := m.writeJSON(bootPath, defaultBootConfig()); err != nil {
			return "", fmt.Errorf("seed boot.json: %w", err)
		}
	}
	raw, err := m.fs.LoadFile(bootPath)
	if err != nil {
		return "", fmt.Errorf("read boot.json: %w", err)
	}
	boot, changed, err := parseBootConfig(raw)
	if err != nil {
		return "", fmt.Errorf("parse boot.json: %w", err)
	}
	if changed {
		if err := m.writeJSON(bootPath, boot); err != nil {
			m.log.Warn("rewrite boot.json failed", "err", err)
		}
	}
	if boot.ActiveProfile == "" {
		boot.ActiveProfile = defaultProfileName
	}
	return boot.ActiveProfile, nil
}

func (m *Manager) setConfigPath(profileFilename string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configPath = m.fs.ResolvePath(configDirName, profileFilename)
	m.cached = nil
	m.virtualStructure = nil
	m.virtualStructureBuilt = time.Time{}
	m.log.Info("config path set", "path", m.configPath)
}

// ─────────────────────────────────────────────────────────────────────
// Load / save / update
// ─────────────────────────────────────────────────────────────────────

// LoadUserConfig returns the cached config or loads it from disk.
// Triggers a virtual-structure rebuild and journal sync on first load.
func (m *Manager) LoadUserConfig() (*Config, error) {
	m.mu.RLock()
	if m.cached != nil && m.virtualStructure != nil {
		c := *m.cached
		m.mu.RUnlock()
		return &c, nil
	}
	m.mu.RUnlock()

	return m.reload()
}

func (m *Manager) reload() (*Config, error) {
	if err := m.ensureUserConfigFile(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	path := m.configPath
	m.mu.RUnlock()

	raw, err := m.fs.LoadFile(path)
	if err != nil {
		// On read failure, fall back to defaults (mirrors JS behavior).
		m.log.Warn("load user config failed; using defaults", "err", err)
		def := defaultConfig()
		m.persist(&def, false)
		return &def, nil
	}

	cfg, changed, err := parseUserConfig(raw)
	if err != nil {
		m.log.Warn("parse user config failed; using defaults", "err", err)
		def := defaultConfig()
		m.persist(&def, false)
		return &def, nil
	}

	if changed {
		if err := m.writeJSON(path, cfg); err != nil {
			m.log.Warn("rewrite repaired config failed", "err", err)
		}
		m.log.Info("repaired missing config fields")
	}

	m.persist(&cfg, true)
	return &cfg, nil
}

// persist replaces the cached config and rebuilds the VFS + syncs journal.
// rebuildVFS=false skips the rebuild (used by reload's error fallback to
// avoid triggering FS work on a freshly-defaulted config — let the next
// access trigger it).
func (m *Manager) persist(cfg *Config, rebuildVFS bool) {
	m.mu.Lock()
	prevCtx := ""
	if m.cached != nil {
		prevCtx = m.cached.ContextFolder
	}
	m.cached = cfg
	if rebuildVFS || prevCtx != cfg.ContextFolder {
		vfs := m.buildVirtualStructure(cfg)
		m.virtualStructure = vfs
		m.virtualStructureBuilt = m.nowFn()
	}
	m.mu.Unlock()

	m.syncJournal(cfg)
}

// UpdateUserConfig merges a partial map into the current config and saves.
// Top-level keys are replaced wholesale (mirrors JS shallow-merge).
func (m *Manager) UpdateUserConfig(partial map[string]any) (*Config, error) {
	cur, err := m.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	curBytes, err := json.Marshal(cur)
	if err != nil {
		return nil, fmt.Errorf("marshal current: %w", err)
	}
	curMap := map[string]any{}
	if err := json.Unmarshal(curBytes, &curMap); err != nil {
		return nil, fmt.Errorf("roundtrip current: %w", err)
	}
	maps.Copy(curMap, partial)
	mergedBytes, err := json.Marshal(curMap)
	if err != nil {
		return nil, fmt.Errorf("marshal merged: %w", err)
	}

	merged := defaultConfig()
	if err := json.Unmarshal(mergedBytes, &merged); err != nil {
		return nil, fmt.Errorf("unmarshal merged: %w", err)
	}

	m.mu.RLock()
	path := m.configPath
	m.mu.RUnlock()
	if err := m.writeJSON(path, &merged); err != nil {
		return nil, fmt.Errorf("save user config: %w", err)
	}

	// Detect context change before persisting so VFS rebuilds correctly.
	prevCtx := cur.ContextFolder
	m.persist(&merged, prevCtx != merged.ContextFolder)

	return &merged, nil
}

func (m *Manager) ensureUserConfigFile() error {
	m.mu.RLock()
	path := m.configPath
	m.mu.RUnlock()
	if path == "" {
		return errors.New("config path not set; call NewManager first")
	}
	if m.fs.FileExists(path) {
		return nil
	}
	def := defaultConfig()
	if err := m.writeJSON(path, def); err != nil {
		return fmt.Errorf("seed user config: %w", err)
	}
	m.log.Info("created user config with defaults", "path", path)
	return nil
}

// ─────────────────────────────────────────────────────────────────────
// Cache invalidation
// ─────────────────────────────────────────────────────────────────────

// InvalidateConfigCache forgets the cached config and VFS. Next access reloads.
func (m *Manager) InvalidateConfigCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cached = nil
	m.virtualStructure = nil
	m.virtualStructureBuilt = time.Time{}
}

// DirtyVirtualStructure marks the VFS stale without dropping the cached config.
// Called by other modules after FS mutations under the context folder.
func (m *Manager) DirtyVirtualStructure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.virtualStructureBuilt = time.Time{}
}

// ─────────────────────────────────────────────────────────────────────
// Virtual structure
// ─────────────────────────────────────────────────────────────────────

// GetVirtualStructure returns a fresh VFS view, rebuilding if cache is stale.
func (m *Manager) GetVirtualStructure() (*VirtualStructure, error) {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	stale := m.virtualStructure == nil || m.nowFn().Sub(m.virtualStructureBuilt) > m.ttl
	m.mu.RUnlock()

	if stale {
		vfs := m.buildVirtualStructure(cfg)
		m.mu.Lock()
		m.virtualStructure = vfs
		m.virtualStructureBuilt = m.nowFn()
		m.mu.Unlock()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.virtualStructure == nil {
		return nil, errors.New("virtual structure unavailable")
	}
	cp := cloneVFS(m.virtualStructure)
	return &cp, nil
}

func (m *Manager) buildVirtualStructure(cfg *Config) *VirtualStructure {
	ctxFolder := cfg.ContextFolder
	if ctxFolder == "" {
		ctxFolder = "./"
	}
	base := m.fs.ResolvePath(ctxFolder)
	templatesPath := filepath.Join(base, templatesDirName)
	storagePath := filepath.Join(base, storageDirName)

	_ = m.fs.EnsureDirectory(templatesPath)
	_ = m.fs.EnsureDirectory(storagePath)

	templateFiles, err := m.fs.ListFiles(templatesPath)
	if err != nil {
		m.log.Warn("list templates failed", "err", err, "path", templatesPath)
		templateFiles = nil
	}

	folders := map[string]TemplateStorageFolder{}
	for _, f := range templateFiles {
		if !strings.HasSuffix(f, templateExt) {
			continue
		}
		name := strings.TrimSuffix(f, templateExt)
		tStoragePath := filepath.Join(storagePath, name)
		imagesPath := filepath.Join(tStoragePath, imagesDirName)

		_ = m.fs.EnsureDirectory(tStoragePath)
		_ = m.fs.EnsureDirectory(imagesPath)

		metaFiles := filterByExt(must(m.fs.ListFiles(tStoragePath)), formExt)
		imageFiles := must(m.fs.ListFiles(imagesPath))

		folders[name] = TemplateStorageFolder{
			Name:       name,
			Filename:   f,
			Path:       tStoragePath,
			MetaFiles:  metaFiles,
			ImageFiles: imageFiles,
		}
	}

	return &VirtualStructure{
		Context:                base,
		Templates:              templatesPath,
		Storage:                storagePath,
		TemplateStorageFolders: folders,
	}
}

// ─────────────────────────────────────────────────────────────────────
// Path / VFS accessors
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) GetContextPath() (string, error) {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return "", err
	}
	ctxFolder := cfg.ContextFolder
	if ctxFolder == "" {
		ctxFolder = "./"
	}
	abs := m.fs.ResolvePath(ctxFolder)
	if err := m.fs.EnsureDirectory(abs); err != nil {
		return "", fmt.Errorf("ensure context dir: %w", err)
	}
	return abs, nil
}

func (m *Manager) GetContextTemplatesPath() (string, error) {
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return "", err
	}
	return vfs.Templates, nil
}

func (m *Manager) GetContextStoragePath() (string, error) {
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return "", err
	}
	return vfs.Storage, nil
}

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
		return &info
	}
	return nil
}

func (m *Manager) GetTemplateStoragePath(templateFilename string) string {
	if info := m.GetTemplateStorageInfo(templateFilename); info != nil {
		return info.Path
	}
	return ""
}

func (m *Manager) GetTemplateMetaFiles(templateFilename string) []string {
	if info := m.GetTemplateStorageInfo(templateFilename); info != nil {
		return info.MetaFiles
	}
	return []string{}
}

func (m *Manager) GetTemplateImageFiles(templateFilename string) []string {
	if info := m.GetTemplateStorageInfo(templateFilename); info != nil {
		return info.ImageFiles
	}
	return []string{}
}

func (m *Manager) GetSingleTemplateEntry(templateName string) *SingleTemplateEntry {
	if templateName == "" {
		return nil
	}
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		return nil
	}
	info, ok := vfs.TemplateStorageFolders[templateName]
	if !ok {
		return nil
	}
	return &SingleTemplateEntry{
		ID:         "template:" + templateName,
		Name:       info.Name,
		Filename:   info.Filename,
		Path:       info.Path,
		MetaFiles:  info.MetaFiles,
		ImageFiles: info.ImageFiles,
	}
}

// ─────────────────────────────────────────────────────────────────────
// Profiles
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) SwitchUserProfile(profileFilename string) (*Config, error) {
	if profileFilename == "" {
		return nil, errors.New("missing profile filename")
	}
	bootData := BootConfig{ActiveProfile: profileFilename}
	if err := m.writeJSON(m.bootPath(), bootData); err != nil {
		return nil, fmt.Errorf("save boot.json: %w", err)
	}
	m.setConfigPath(profileFilename)
	return m.LoadUserConfig()
}

func (m *Manager) ListAvailableProfiles() ([]ProfileEntry, error) {
	configDir := m.fs.ResolvePath(configDirName)
	files, err := m.fs.ListFiles(configDir)
	if err != nil {
		return nil, fmt.Errorf("list config dir: %w", err)
	}
	out := make([]ProfileEntry, 0, len(files))
	for _, f := range files {
		if !strings.HasSuffix(f, ".json") || f == bootFileName {
			continue
		}
		display := "(unknown)"
		raw, err := m.fs.LoadFile(filepath.Join(configDir, f))
		if err == nil {
			cfg, _, perr := parseUserConfig(raw)
			if perr == nil {
				switch {
				case strings.TrimSpace(cfg.ProfileName) != "":
					display = strings.TrimSpace(cfg.ProfileName)
				case strings.TrimSpace(cfg.AuthorName) != "":
					display = strings.TrimSpace(cfg.AuthorName)
				default:
					display = "(unnamed)"
				}
			} else {
				m.log.Warn("could not read profile", "file", f, "err", perr)
			}
		}
		out = append(out, ProfileEntry{Value: f, Display: display})
	}
	return out, nil
}

func (m *Manager) CurrentProfileFilename() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.configPath == "" {
		return ""
	}
	return filepath.Base(m.configPath)
}

func (m *Manager) ExportUserProfile(profileFilename, targetPath string, overwrite bool) ProfileResult {
	if profileFilename == "" || targetPath == "" {
		return ProfileResult{Success: false, Error: "Missing profileFilename or targetPath."}
	}
	source := m.fs.ResolvePath(configDirName, profileFilename)
	if !m.fs.FileExists(source) {
		return ProfileResult{
			Success: false,
			Error:   fmt.Sprintf("Profile file not found: %s", profileFilename),
			Code:    "not_found",
		}
	}
	if err := m.fs.EnsureDirectory(filepath.Dir(targetPath)); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}
	if err := m.fs.CopyFile(source, targetPath, overwrite); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}
	m.log.Info("exported profile", "profile", profileFilename, "target", targetPath)
	return ProfileResult{
		Success:         true,
		ProfileFilename: profileFilename,
		SourcePath:      source,
		TargetPath:      targetPath,
	}
}

func (m *Manager) DeleteUserProfile(profileFilename string) ProfileResult {
	if profileFilename == "" {
		return ProfileResult{Success: false, Error: "Missing profileFilename.", Code: "missing_filename"}
	}
	if profileFilename == bootFileName {
		return ProfileResult{Success: false, Error: "boot.json cannot be deleted.", Code: "boot_forbidden"}
	}
	if profileFilename == m.CurrentProfileFilename() {
		return ProfileResult{Success: false, Error: "The active profile cannot be deleted.", Code: "active_profile"}
	}
	target := m.fs.ResolvePath(configDirName, profileFilename)
	if !m.fs.FileExists(target) {
		return ProfileResult{
			Success: false,
			Error:   fmt.Sprintf("Profile file not found: %s", profileFilename),
			Code:    "not_found",
		}
	}
	if err := m.fs.DeleteFile(target); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "delete_failed"}
	}
	m.log.Info("deleted profile", "profile", profileFilename)
	return ProfileResult{Success: true, Filename: profileFilename}
}

func (m *Manager) ImportUserProfile(sourcePath, profileFilename string, overwrite bool) ProfileResult {
	if sourcePath == "" {
		return ProfileResult{Success: false, Error: "Missing sourcePath."}
	}
	if !m.fs.FileExists(sourcePath) {
		return ProfileResult{
			Success: false,
			Error:   fmt.Sprintf("Source file not found: %s", sourcePath),
			Code:    "not_found",
		}
	}
	final := profileFilename
	if final == "" {
		final = normalizeProfileFilename(filepath.Base(sourcePath))
	}
	if final == "" {
		return ProfileResult{Success: false, Error: "Unable to derive a valid profile filename.", Code: "invalid_name"}
	}
	if final == bootFileName {
		return ProfileResult{
			Success: false,
			Error:   "boot.json cannot be imported as a profile.",
			Code:    "boot_forbidden",
		}
	}

	target := m.fs.ResolvePath(configDirName, final)
	if m.fs.FileExists(target) && !overwrite {
		return ProfileResult{
			Success:  false,
			Error:    fmt.Sprintf("Profile already exists: %s", final),
			Code:     "exists",
			Filename: final,
			TargetPath: target,
		}
	}
	if err := m.fs.CopyFile(sourcePath, target, overwrite); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}

	// Validate by re-parsing + re-saving canonical form.
	raw, err := m.fs.LoadFile(target)
	if err != nil {
		return ProfileResult{
			Success: false,
			Error:   "Imported file is not readable.",
			Code:    "invalid_config",
			Filename: final, TargetPath: target,
		}
	}
	cfg, _, err := parseUserConfig(raw)
	if err != nil {
		return ProfileResult{
			Success: false,
			Error:   "Imported file is not a valid Formidable config.",
			Code:    "invalid_config",
			Filename: final, TargetPath: target,
		}
	}
	if err := m.writeJSON(target, cfg); err != nil {
		return ProfileResult{Success: false, Error: err.Error(), Code: "copy_failed"}
	}

	m.InvalidateConfigCache()
	m.log.Info("imported profile", "source", sourcePath, "as", final)
	return ProfileResult{Success: true, Filename: final, TargetPath: target}
}

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := m.fs.EnsureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return m.fs.SaveFile(path, string(b))
}

func (m *Manager) syncJournal(cfg *Config) {
	m.mu.RLock()
	j := m.journal
	m.mu.RUnlock()
	if j == nil {
		return
	}
	if err := j.Configure(cfg.ContextFolder, cfg.RemoteBackend); err != nil {
		m.log.Warn("journal configure failed", "err", err, "context", cfg.ContextFolder)
	}
}

// parseUserConfig deserializes raw JSON, fills defaults for absent fields,
// and reports whether the input was missing fields (to trigger a rewrite).
func parseUserConfig(raw string) (Config, bool, error) {
	cfg := defaultConfig()
	rawMap := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		return cfg, true, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, true, err
	}
	changed := false
	for _, key := range configFieldNames {
		if _, ok := rawMap[key]; !ok {
			changed = true
			break
		}
	}
	return cfg, changed, nil
}

func parseBootConfig(raw string) (BootConfig, bool, error) {
	boot := defaultBootConfig()
	rawMap := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		return boot, true, err
	}
	if err := json.Unmarshal([]byte(raw), &boot); err != nil {
		return boot, true, err
	}
	changed := false
	if _, ok := rawMap["active_profile"]; !ok {
		changed = true
	}
	// Drop the obsolete pending_changes field if present (matches JS schema).
	if _, ok := rawMap["pending_changes"]; ok {
		changed = true
	}
	return boot, changed, nil
}

// configFieldNames lists every JSON tag on Config, used by the
// "missing field" check in parseUserConfig. Hand-maintained to avoid
// reflection in hot paths.
var configFieldNames = []string{
	"profile_name", "theme",
	"show_icon_buttons", "show_paste_buttons",
	"use_expressions", "show_meta_section",
	"loop_state_collapsed", "field_state_collapsed",
	"font_size", "development_enable", "logging_enabled", "enable_plugins",
	"context_mode", "context_folder",
	"selected_template", "selected_data_file",
	"author_name", "author_email", "language", "encryption_key",
	"use_git", "git_root", "git_branch",
	"remote_backend", "gigot_base_url", "gigot_repo_name", "gigot_token",
	"enable_internal_server", "internal_server_port",
	"window_bounds",
	"template_sidebar_width", "storage_sidebar_width",
	"status_buttons", "history",
}

var profileSlugCleanup = regexp.MustCompile(`[^a-z0-9-]+`)
var profileSlugTrim = regexp.MustCompile(`^-+|-+$`)

// normalizeProfileFilename lowercases, strips folder prefix and `.json`
// extension, replaces non-[a-z0-9-] runs with `-`, trims leading/trailing
// dashes, and re-attaches `.json`. Empty input returns "".
func normalizeProfileFilename(name string) string {
	if name == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, ".json")
	base = profileSlugCleanup.ReplaceAllString(base, "-")
	base = profileSlugTrim.ReplaceAllString(base, "")
	if base == "" {
		return ""
	}
	return base + ".json"
}

func cloneVFS(v *VirtualStructure) VirtualStructure {
	out := *v
	folders := make(map[string]TemplateStorageFolder, len(v.TemplateStorageFolders))
	for k, val := range v.TemplateStorageFolders {
		copyFolder := val
		copyFolder.MetaFiles = append([]string(nil), val.MetaFiles...)
		copyFolder.ImageFiles = append([]string(nil), val.ImageFiles...)
		folders[k] = copyFolder
	}
	out.TemplateStorageFolders = folders
	return out
}

func filterByExt(files []string, ext string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if strings.HasSuffix(f, ext) {
			out = append(out, f)
		}
	}
	return out
}

func must(files []string, _ error) []string {
	if files == nil {
		return []string{}
	}
	return files
}
