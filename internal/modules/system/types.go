package system

// FS is the filesystem-facing surface that downstream modules depend on.
// *Manager satisfies this interface.
type FS interface {
	AppRoot() string
	ResolvePath(segments ...string) string
	JoinPath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	DeleteFolder(path string) error
	EmptyFolder(path string) error
	CopyFile(from, to string, overwrite bool) error
	CopyFolder(from, to string, overwrite bool) error
	ListFiles(dir string) ([]string, error)
	ListFolders(dir string) ([]string, error)
	ListDirectoryEntries(dir string) ([]DirEntry, error)
	WalkFiles(dir string) ([]string, error)
}

type DirEntry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
	IsFile      bool   `json:"isFile"`
}

type FetchOptions struct {
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	TimeoutSecs   int               `json:"timeoutSecs"`
	FollowRedirs  bool              `json:"followRedirects"`
}

type FetchResult struct {
	StatusCode int               `json:"statusCode"`
	Body       string            `json:"body"`
	Headers    map[string]string `json:"headers"`
	URL        string            `json:"url"`
}

// JournalEmitter lets the journal module observe filesystem mutations
// without system depending on it. Wired in the composition root.
type JournalEmitter interface {
	RecordOp(op string, path string, meta map[string]any)
}
