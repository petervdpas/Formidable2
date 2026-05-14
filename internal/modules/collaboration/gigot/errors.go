package gigot

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingConn fires when an op is invoked without a Connection
	// (zero value or nil). Connection carries BaseURL + Token at minimum;
	// callers should resolve both from the active profile + keychain
	// before calling Manager methods.
	ErrMissingConn = errors.New("gigot: missing connection")

	// ErrMissingBaseURL fires when a Connection has no BaseURL set.
	ErrMissingBaseURL = errors.New("gigot: missing base url")

	// ErrMissingToken fires when a Connection has no subscription
	// bearer set. Distinct from "401 Unauthorized" — this is the
	// pre-flight check before any HTTP request leaves the client.
	ErrMissingToken = errors.New("gigot: missing subscription token")

	// ErrMissingRepo fires when an op requires a repo-scoped Connection
	// (anything other than Ping / Me) and RepoName is empty.
	ErrMissingRepo = errors.New("gigot: missing repo name")

	// ErrMissingContext fires when an orchestration op (PushLocal /
	// PullLocal / Sync) is invoked without a context folder.
	ErrMissingContext = errors.New("gigot: missing context folder")

	// ErrMissingDestinationID fires when DestinationSync is called
	// without a destination ID — gigot's mirror-sync targets are
	// id-keyed, so a blank id makes no sense.
	ErrMissingDestinationID = errors.New("gigot: missing destination id")

	// ErrEmptyContext fires when the walker finds no Formidable-managed
	// files in the context folder — pushing nothing is almost certainly
	// a misconfigured path rather than an intentional no-op.
	ErrEmptyContext = errors.New("gigot: no formidable files in context folder")

	// ErrNoParentVersion fires when HEAD is asked for on a repo that
	// has no commits yet — gigot returns 409 in that case and a push
	// needs an alternate first-commit path (not yet implemented).
	ErrNoParentVersion = errors.New("gigot: remote repo has no HEAD")

	// ErrNotImplemented is the scaffold-stage sentinel for methods
	// declared but not yet wired up. Production code paths must not
	// return this — tests catch any leak via assertion on the error
	// value rather than its string.
	ErrNotImplemented = errors.New("gigot: not implemented")
)

// HTTPError wraps a non-2xx response from a gigot server. Status is the
// HTTP code; Body is the raw response body for debugging; Path lets the
// caller surface "GET /api/...: 401 Unauthorized" without re-deriving
// the route. Carries (rather than wraps) the route so the original
// errors.Is(err, ErrSomething) sentinels still match where they should.
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
