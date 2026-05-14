package gigot

import (
	"net/http"
	"strconv"
)

// Ping issues GET /api/health. Tolerates a missing RepoName since
// /health is repo-agnostic.
func (m *Manager) Ping(conn Connection) (*HealthResponse, error) {
	if err := validateConn(conn, false); err != nil {
		return nil, err
	}
	var out HealthResponse
	if err := m.do(http.MethodGet, conn, "/api/health", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Me issues GET /api/me — bearer-aware self-introspection. Repo-agnostic.
func (m *Manager) Me(conn Connection) (*MeResponse, error) {
	if err := validateConn(conn, false); err != nil {
		return nil, err
	}
	var out MeResponse
	if err := m.do(http.MethodGet, conn, "/api/me", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Context issues GET /api/repos/{repo}/context — per-repo bootstrap.
func (m *Manager) Context(conn Connection) (*RepoContextResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out RepoContextResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/context"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Formidable issues GET /api/repos/{repo}/formidable — Formidable-shape
// bootstrap (marker + templates + storage summary).
func (m *Manager) Formidable(conn Connection) (*RepoFormidableResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out RepoFormidableResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/formidable"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Head issues GET /api/repos/{repo}/head. Returns 409 for an empty
// repo with no commits — surfaced verbatim as *HTTPError so the
// caller's first-write path can detect "remote has no HEAD yet."
func (m *Manager) Head(conn Connection) (*HeadResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out HeadResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/head"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Tree issues GET /api/repos/{repo}/tree — recursive file listing at HEAD.
func (m *Manager) Tree(conn Connection) (*TreeResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out TreeResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/tree"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFile issues GET /api/repos/{repo}/files/{path}. repoRelPath may
// be multiple slash-separated segments; encodeSegments preserves the
// slashes while URL-encoding each segment.
func (m *Manager) GetFile(conn Connection, repoRelPath string) (*FileResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out FileResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/files/" + encodeSegments(repoRelPath)
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Log issues GET /api/repos/{repo}/log?limit=N. limit<=0 omits the
// query so the server falls back to its default page size.
func (m *Manager) Log(conn Connection, limit int) ([]LogEntry, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var query map[string]string
	if limit > 0 {
		query = map[string]string{"limit": strconv.Itoa(limit)}
	}
	var out []LogEntry
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/log"
	if err := m.do(http.MethodGet, conn, path, query, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Destinations issues GET /api/repos/{repo}/destinations — mirror-sync
// targets attached to this repo.
func (m *Manager) Destinations(conn Connection) ([]Destination, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out []Destination
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/destinations"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DestinationSync issues POST /api/repos/{repo}/destinations/{id}/sync —
// manual retry of a mirror push. Synchronous on the server.
func (m *Manager) DestinationSync(conn Connection, destinationID string) (*Destination, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	if destinationID == "" {
		return nil, ErrMissingDestinationID
	}
	var out Destination
	path := "/api/repos/" + encodeSegment(conn.RepoName) +
		"/destinations/" + encodeSegment(destinationID) + "/sync"
	if err := m.do(http.MethodPost, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Commit issues POST /api/repos/{repo}/commits — atomic multi-file
// commit. ParentVersion must match current HEAD or the server returns
// 409 (surfaced as *HTTPError).
func (m *Manager) Commit(conn Connection, req CommitRequest) (*CommitResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out CommitResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/commits"
	if err := m.do(http.MethodPost, conn, path, nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
