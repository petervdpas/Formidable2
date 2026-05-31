package gigot

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingConn fires when an op is invoked without a Connection.
	ErrMissingConn = errors.New("gigot: missing connection")

	// ErrMissingBaseURL fires when a Connection has no BaseURL set.
	ErrMissingBaseURL = errors.New("gigot: missing base url")

	// ErrMissingToken is the pre-flight check for a missing subscription bearer, distinct from a server 401.
	ErrMissingToken = errors.New("gigot: missing subscription token")

	// ErrMissingRepo fires when a repo-scoped op has an empty RepoName.
	ErrMissingRepo = errors.New("gigot: missing repo name")

	// ErrMissingContext fires when an orchestration op has no context folder.
	ErrMissingContext = errors.New("gigot: missing context folder")

	// ErrMissingDestinationID fires when DestinationSync is called without a destination ID.
	ErrMissingDestinationID = errors.New("gigot: missing destination id")

	// ErrEmptyContext fires when the walker finds no Formidable-managed files (likely a misconfigured path).
	ErrEmptyContext = errors.New("gigot: no formidable files in context folder")

	// ErrNoParentVersion fires when the remote repo has no commits yet (gigot returns 409); first-commit path not yet implemented.
	ErrNoParentVersion = errors.New("gigot: remote repo has no HEAD")

	// ErrNotImplemented is the scaffold sentinel; production paths must not return it.
	ErrNotImplemented = errors.New("gigot: not implemented")
)

// HTTPError wraps a non-2xx gigot response. It carries (not wraps) the route so errors.Is sentinels still match.
type HTTPError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("gigot: %s %s: %d %s", e.Method, e.Path, e.Status, e.Body)
}
