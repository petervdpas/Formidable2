package gigot

import (
	"net/http"
	"strconv"
)

// Ping issues GET /api/health; tolerates a missing RepoName (repo-agnostic).
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

// Me issues GET /api/me (bearer-aware, repo-agnostic).
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

// Context issues GET /api/repos/{repo}/context.
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

// Formidable issues GET /api/repos/{repo}/formidable (marker + templates + storage summary).
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

// Head issues GET /api/repos/{repo}/head; an empty repo returns 409 (as *HTTPError) so the first-write path can detect "no HEAD yet".
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

// Tree issues GET /api/repos/{repo}/tree (recursive file listing at HEAD).
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

// GetFile issues GET /api/repos/{repo}/files/{path}; encodeSegments preserves slashes while URL-encoding each segment.
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

// Log issues GET /api/repos/{repo}/log[?limit=N&with_changes=1]; limit<=0 omits the query (server default page size).
func (m *Manager) Log(conn Connection, limit int, withChanges bool) (*RepoLogResponse, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	query := map[string]string{}
	if limit > 0 {
		query["limit"] = strconv.Itoa(limit)
	}
	if withChanges {
		query["with_changes"] = "1"
	}
	if len(query) == 0 {
		query = nil
	}
	var out RepoLogResponse
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/log"
	if err := m.do(http.MethodGet, conn, path, query, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Destinations issues GET /api/repos/{repo}/destinations, unwrapping the server's {destinations, count} envelope to a slice.
func (m *Manager) Destinations(conn Connection) ([]Destination, error) {
	if err := validateConn(conn, true); err != nil {
		return nil, err
	}
	var out struct {
		Destinations []Destination `json:"destinations"`
		Count        int           `json:"count"`
	}
	path := "/api/repos/" + encodeSegment(conn.RepoName) + "/destinations"
	if err := m.do(http.MethodGet, conn, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Destinations, nil
}

// DestinationSync issues POST /api/repos/{repo}/destinations/{id}/sync (manual mirror-push retry, synchronous server-side).
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

// Commit issues POST /api/repos/{repo}/commits (atomic multi-file); a ParentVersion that doesn't match HEAD returns 409 (as *HTTPError).
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
