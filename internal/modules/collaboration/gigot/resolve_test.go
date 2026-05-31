package gigot

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

const conflictRecordPath = "storage/notes/a.meta.json"
const conflictFileRoute = "/api/repos/r/files/storage/notes/a.meta.json"

func conn(srvURL string) Connection {
	return Connection{BaseURL: srvURL, Token: "t", RepoName: "r"}
}

// ConflictValues reads yours from disk and fetches theirs from the server,
// returning both candidate values per conflicting field for the picker UI.
func TestConflictValues_ReturnsYoursAndTheirs(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, conflictRecordPath, `{"meta":{},"data":{"name":"Yours"}}`)

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", conflictFileRoute, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			ContentB64: base64.StdEncoding.EncodeToString([]byte(`{"meta":{},"data":{"name":"Theirs"}}`)),
		})
	})
	m := newOrchestrationManager(t, srv)

	conflicts := []PathConflict{{Path: conflictRecordPath, Fields: []FieldConflict{{Scope: "data", Key: "name"}}}}
	vals, err := m.ConflictValues(conn(srv.URL), ctxDir, conflicts)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 {
		t.Fatalf("got %d values, want 1", len(vals))
	}
	if vals[0].Yours != `"Yours"` || vals[0].Theirs != `"Theirs"` {
		t.Errorf("got yours=%s theirs=%s", vals[0].Yours, vals[0].Theirs)
	}
}

// "Take theirs" neutralizes the conflicting field to the server's value and
// pushes once with the ledger base as parent, so the server 3-way merges.
func TestResolveConflicts_TakeTheirs_NeutralizesAndPushesOnce(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, conflictRecordPath, `{"meta":{},"data":{"name":"Yours"}}`)
	fs := newFakeFS()
	m0 := NewManager(fs)
	seed := EmptyTrackRecord()
	seed.Version = "baseV"
	seed.Files[conflictRecordPath] = "oldsha"
	if err := m0.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	h.handle("GET", conflictFileRoute, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			ContentB64: base64.StdEncoding.EncodeToString([]byte(`{"meta":{},"data":{"name":"Theirs"}}`)),
		})
	})
	var commits []CommitRequest
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		var req CommitRequest
		readJSONBody(t, r, &req)
		commits = append(commits, req)
		_ = json.NewEncoder(w).Encode(CommitResponse{Version: "mergedV"})
	})
	m := NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.ResolveConflicts(conn(srv.URL), ctxDir, "resolve",
		[]FieldResolution{{Path: conflictRecordPath, Scope: "data", Key: "name", Side: "theirs"}})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Conflicts) != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
	if len(commits) != 1 {
		t.Fatalf("take-theirs must push exactly once, got %d", len(commits))
	}
	if commits[0].ParentVersion != "baseV" {
		t.Errorf("push parent = %s, want ledger base baseV", commits[0].ParentVersion)
	}
	content, _ := base64.StdEncoding.DecodeString(commits[0].Changes[0].ContentB64)
	if v, _, _ := getRecordField(content, "data", "name"); string(v) != `"Theirs"` {
		t.Errorf("pushed name = %s, want neutralized to Theirs", v)
	}
}

// "Keep mine" pushes twice: first neutralized to theirs (server merges,
// parent=base), then a fast-forward (parent=new head) carrying my value.
func TestResolveConflicts_KeepMine_FastForwardsMyValue(t *testing.T) {
	ctxDir := t.TempDir()
	writeFile(t, ctxDir, conflictRecordPath, `{"meta":{},"data":{"name":"Yours"}}`)
	fs := newFakeFS()
	m0 := NewManager(fs)
	seed := EmptyTrackRecord()
	seed.Version = "baseV"
	seed.Files[conflictRecordPath] = "oldsha"
	if err := m0.WriteTrackRecord(ctxDir, seed); err != nil {
		t.Fatal(err)
	}

	srv, h := newOrchestrationServer(t)
	defer srv.Close()
	// The server's merged record after push #1 has name=Theirs (the value we
	// neutralized to); push #2 must override it back to Yours.
	mergedAfterPush1 := `{"meta":{},"data":{"name":"Theirs"}}`
	h.handle("GET", conflictFileRoute, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(FileResponse{
			ContentB64: base64.StdEncoding.EncodeToString([]byte(mergedAfterPush1)),
		})
	})
	var commits []CommitRequest
	h.handle("POST", "/api/repos/r/commits", func(w http.ResponseWriter, r *http.Request) {
		var req CommitRequest
		readJSONBody(t, r, &req)
		commits = append(commits, req)
		ver := "h2"
		if len(commits) == 2 {
			ver = "h3"
		}
		_ = json.NewEncoder(w).Encode(CommitResponse{Version: ver})
	})
	m := NewManager(fs, WithHTTPClient(srv.Client()))

	res, err := m.ResolveConflicts(conn(srv.URL), ctxDir, "resolve",
		[]FieldResolution{{Path: conflictRecordPath, Scope: "data", Key: "name", Side: "mine"}})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Conflicts) != 0 {
		t.Fatalf("unexpected result %+v", res)
	}
	if len(commits) != 2 {
		t.Fatalf("keep-mine must push twice, got %d", len(commits))
	}
	// Push #1 neutralized to theirs, parent = base.
	if commits[0].ParentVersion != "baseV" {
		t.Errorf("push #1 parent = %s, want baseV", commits[0].ParentVersion)
	}
	c1, _ := base64.StdEncoding.DecodeString(commits[0].Changes[0].ContentB64)
	if v, _, _ := getRecordField(c1, "data", "name"); string(v) != `"Theirs"` {
		t.Errorf("push #1 name = %s, want Theirs", v)
	}
	// Push #2 fast-forwards my value onto the new head.
	if commits[1].ParentVersion != "h2" {
		t.Errorf("push #2 parent = %s, want new head h2 (fast-forward)", commits[1].ParentVersion)
	}
	c2, _ := base64.StdEncoding.DecodeString(commits[1].Changes[0].ContentB64)
	if v, _, _ := getRecordField(c2, "data", "name"); string(v) != `"Yours"` {
		t.Errorf("push #2 name = %s, want my value Yours", v)
	}
}
