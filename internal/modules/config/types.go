// Package config owns the user configuration, boot profile, and the
// derived Virtual File System (VFS) view of the on-disk Formidable
// data layout. Wails-only — no HTTP handlers; raw config is too
// sensitive even for the loopback API.
package config

// Config mirrors `Formidable/schemas/config.schema.js` exactly.
// Field names and JSON tags are preserved so existing user.json files
// load without migration.
type Config struct {
	ProfileName          string         `json:"profile_name"`
	Theme                string         `json:"theme"`
	ShowPasteButtons     bool           `json:"show_paste_buttons"`
	UseExpressions       bool           `json:"use_expressions"`
	ShowMetaSection      bool           `json:"show_meta_section"`
	LoopStateCollapsed   bool           `json:"loop_state_collapsed"`
	FieldStateCollapsed  bool           `json:"field_state_collapsed"`
	FontSize             int            `json:"font_size"`
	DevelopmentEnable    bool           `json:"development_enable"`
	LoggingEnabled       bool           `json:"logging_enabled"`
	EnablePlugins        bool           `json:"enable_plugins"`
	ContextMode          string         `json:"context_mode"`
	ContextRibbon        string         `json:"context_ribbon"`
	ContextFolder        string         `json:"context_folder"`
	SelectedTemplate     string         `json:"selected_template"`
	SelectedDataFile     string         `json:"selected_data_file"`
	AuthorName           string         `json:"author_name"`
	AuthorEmail          string         `json:"author_email"`
	Language             string         `json:"language"`
	RemoteBackend        string         `json:"remote_backend"`
	GitRoot              string         `json:"git_root"`
	GitBranch            string         `json:"git_branch"`
	GigotBaseURL         string         `json:"gigot_base_url"`
	GigotRepoName        string         `json:"gigot_repo_name"`
	GigotToken           string         `json:"gigot_token"`
	EnableInternalServer bool           `json:"enable_internal_server"`
	InternalServerPort   int            `json:"internal_server_port"`
	WindowBounds  WindowBounds  `json:"window_bounds"`
	SidebarWidth  int           `json:"sidebar_width"`
	StatusButtons StatusButtons `json:"status_buttons"`
	History       History       `json:"history"`
}

// WindowBounds — X/Y are pointers so absent (centered) is distinguishable
// from an explicit (0,0) position.
type WindowBounds struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         *int `json:"x,omitempty"`
	Y         *int `json:"y,omitempty"`
	Maximized bool `json:"maximized,omitempty"`
}

type StatusButtons struct {
	Reloader   bool `json:"reloader"`
	Charpicker bool `json:"charpicker"`
	Gitquick   bool `json:"gitquick"`
	Gigotload  bool `json:"gigotload"`
}

type History struct {
	Enabled bool  `json:"enabled"`
	Persist bool  `json:"persist"`
	MaxSize int   `json:"max_size"`
	Stack   []string `json:"stack"`
	Index   int   `json:"index"`
}

// BootConfig points at the active profile and is stored in config/boot.json.
type BootConfig struct {
	ActiveProfile string `json:"active_profile"`
}

// VirtualStructure is the materialised view of the on-disk Formidable
// layout under <context_folder>. Mirrors `configManager.buildVirtualStructure`
// in the Electron app. Per-template storage folders are auto-created and
// scanned for `.meta.json` (forms) and image files.
type VirtualStructure struct {
	Context                string                            `json:"context"`
	Templates              string                            `json:"templates"`
	Storage                string                            `json:"storage"`
	TemplateStorageFolders map[string]TemplateStorageFolder `json:"templateStorageFolders"`
}

// TemplateStorageFolder is one entry in VirtualStructure.TemplateStorageFolders.
// Name is the bare template id (e.g. "basic"); Filename is the YAML file ("basic.yaml").
type TemplateStorageFolder struct {
	Name       string   `json:"name"`
	Filename   string   `json:"filename"`
	Path       string   `json:"path"`
	MetaFiles  []string `json:"metaFiles"`
	ImageFiles []string `json:"imageFiles"`
}

// SingleTemplateEntry wraps a TemplateStorageFolder with an "id" field
// for use by frontend list managers.
type SingleTemplateEntry struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Filename   string   `json:"filename"`
	Path       string   `json:"path"`
	MetaFiles  []string `json:"metaFiles"`
	ImageFiles []string `json:"imageFiles"`
}

// ProfileEntry is one row in the profile picker UI.
type ProfileEntry struct {
	Value   string `json:"value"`   // filename (e.g. "user.json")
	Display string `json:"display"` // profile_name | author_name | "(unnamed)"
}

// ProfileResult is the shared result shape for export/import/delete profile
// operations. Mirrors the {success, error, code, ...} pattern from JS so
// frontend modal handlers don't need branching per call.
type ProfileResult struct {
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	Code            string `json:"code,omitempty"`
	ProfileFilename string `json:"profileFilename,omitempty"`
	SourcePath      string `json:"sourcePath,omitempty"`
	TargetPath      string `json:"targetPath,omitempty"`
	Filename        string `json:"filename,omitempty"`
}

// JournalConfigurer lets the journal module observe config changes
// without config depending on it. Wired in `internal/app/app.go`.
// Nil journal is treated as a no-op throughout.
//
// Init/baseline seeding is intentionally NOT part of this interface —
// the composition root calls journal.Manager.Init() once at startup
// and again on profile/context switches, since the timing rules differ
// from "every config load."
type JournalConfigurer interface {
	Configure(contextFolder, backend string) error
}
