package dialog

// FileFilter narrows what the native picker shows. Pattern uses the
// platform-native glob (e.g. `*.json`); on macOS the Wails layer
// translates this into UTI/extensions internally.
type FileFilter struct {
	DisplayName string `json:"displayName"`
	Pattern     string `json:"pattern"`
}
