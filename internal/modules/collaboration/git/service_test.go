package git

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/petervdpas/formidable2/internal/modules/journal"
)

// ─────────────────────────────────────────────────────────────────────
// Fakes
// ─────────────────────────────────────────────────────────────────────

type syncCall struct {
	backend, version string
	pushed, pulled   int
}

type seenCall struct {
	backend, version string
}

type fakeJournal struct {
	mu    sync.Mutex
	syncs []syncCall
	seens []seenCall
	// pending is keyed by backend → list of pending changes returned
	// by Pending(). Tests configure this directly to drive
	// PullWithStash's snapshot manifest. Nil/missing key returns empty.
	pending map[string][]journal.PendingChange
}

func (f *fakeJournal) RecordSync(backend, version string, pushed, pulled int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs = append(f.syncs, syncCall{backend, version, pushed, pulled})
}

func (f *fakeJournal) RecordRemoteSeen(backend, version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seens = append(f.seens, seenCall{backend, version})
}

func (f *fakeJournal) Pending(backend string) journal.PendingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	paths := f.pending[backend]
	out := make([]journal.PendingChange, len(paths))
	copy(out, paths)
	return journal.PendingResult{Count: len(out), Paths: out}
}

func (f *fakeJournal) setPending(backend string, paths []journal.PendingChange) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending == nil {
		f.pending = map[string][]journal.PendingChange{}
	}
	f.pending[backend] = paths
}

func (f *fakeJournal) snapshot() ([]syncCall, []seenCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	syncs := make([]syncCall, len(f.syncs))
	copy(syncs, f.syncs)
	seens := make([]seenCall, len(f.seens))
	copy(seens, f.seens)
	return syncs, seens
}

// fakeProfile / fakeCreds stand in for the credential + profile readers
// in scenarios where we don't care about PAT resolution.
type fakeProfile struct{ name string }

func (f *fakeProfile) CurrentProfileFilename() string { return f.name }

type fakeCreds struct{ secret string }

func (f *fakeCreds) Get(string) (string, error) { return f.secret, nil }

// newServiceWithJournal builds a Service wired to a fakeJournal but with
// nil credential/profile readers - Service.resolvePAT collapses to ""
// when those are nil, which is fine for these tests (they don't assert
// anything about auth).
func newServiceWithJournal(t *testing.T) (*Service, *fakeJournal) {
	t.Helper()
	jrnl := &fakeJournal{}
	svc := NewService(NewManager(), nil, nil, jrnl)
	return svc, jrnl
}

// ─────────────────────────────────────────────────────────────────────
// Push happy paths
// ─────────────────────────────────────────────────────────────────────

func TestService_Push_AdvancingRecordsSync(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "new.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, _ := wr.Worktree()
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("local", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	res, err := svc.Push(PushOptions{Path: work})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res == nil || res.AlreadyUpToDate {
		t.Fatalf("expected advancing push, got %+v", res)
	}

	syncs, seens := jrnl.snapshot()
	if len(syncs) != 1 {
		t.Fatalf("expected 1 sync call, got %d (%+v)", len(syncs), syncs)
	}
	if syncs[0].backend != "git" {
		t.Errorf("sync backend = %q, want \"git\"", syncs[0].backend)
	}
	if syncs[0].version != res.NewHead {
		t.Errorf("sync version = %q, want %q (NewHead)", syncs[0].version, res.NewHead)
	}
	if syncs[0].pushed != 1 || syncs[0].pulled != 0 {
		t.Errorf("sync counts = (%d,%d), want (1,0)", syncs[0].pushed, syncs[0].pulled)
	}
	if len(seens) != 0 {
		t.Errorf("advancing push should not RecordRemoteSeen, got %+v", seens)
	}
}

func TestService_Push_AlreadyUpToDateRecordsRemoteSeen(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	res, err := svc.Push(PushOptions{Path: work})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res == nil || !res.AlreadyUpToDate {
		t.Fatalf("expected ATU, got %+v", res)
	}

	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 {
		t.Errorf("ATU push should not RecordSync, got %+v", syncs)
	}
	if len(seens) != 1 {
		t.Fatalf("expected 1 remote-seen call, got %d", len(seens))
	}
	if seens[0].backend != "git" || seens[0].version != res.NewHead {
		t.Errorf("seen = %+v, want backend=git version=%q", seens[0], res.NewHead)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Push unhappy paths - error means NO journal entry ever
// ─────────────────────────────────────────────────────────────────────

func TestService_Push_EmptyPath_NoJournalCall(t *testing.T) {
	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Push(PushOptions{Path: ""}); err == nil {
		t.Fatal("expected error for empty path")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("error path leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Push_NonRepoPath_NoJournalCall(t *testing.T) {
	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Push(PushOptions{Path: t.TempDir()}); err == nil {
		t.Fatal("expected error for non-repo path")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("non-repo error leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Push_DetachedHEAD_NoJournalCall(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	h, _ := wr.Head()
	// Detach HEAD by replacing the symbolic ref with a hash ref.
	if err := wr.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, h.Hash())); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Push(PushOptions{Path: work}); err == nil {
		t.Fatal("expected detached-HEAD error")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("detached error leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Push_AuthFailure_NoJournalCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("WWW-Authenticate", `Basic realm="Git"`)
		rw.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// pullAuthFixture builds a non-bare repo with a HEAD commit and an
	// origin remote - exactly what Push needs for the auth handshake.
	work := pullAuthFixture(t, srv.URL+"/repo.git")

	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Push(PushOptions{Path: work, PAT: "bad"}); err == nil {
		t.Fatal("expected auth error")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("auth error leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Pull happy paths
// ─────────────────────────────────────────────────────────────────────

func TestService_Pull_AdvancingRecordsRemoteSeen(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	// Advance the bare so the pull has something to apply.
	advance := t.TempDir()
	pr, err := gogit.PlainClone(advance, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(advance, "remote.txt"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	pwt, _ := pr.Worktree()
	if _, err := pwt.Add("remote.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := pwt.Commit("rs", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	res, err := svc.Pull(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res == nil || res.AlreadyUpToDate {
		t.Fatalf("expected advancing pull, got %+v", res)
	}

	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 {
		t.Errorf("Pull must not RecordSync (inbound op), got %+v", syncs)
	}
	if len(seens) != 1 {
		t.Fatalf("expected 1 remote-seen, got %d", len(seens))
	}
	if seens[0].backend != "git" || seens[0].version != res.NewHead {
		t.Errorf("seen = %+v, want backend=git version=%q", seens[0], res.NewHead)
	}
}

func TestService_Pull_AlreadyUpToDateRecordsRemoteSeen(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	res, err := svc.Pull(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res == nil || !res.AlreadyUpToDate {
		t.Fatalf("expected ATU, got %+v", res)
	}

	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 {
		t.Errorf("Pull must not RecordSync, got %+v", syncs)
	}
	if len(seens) != 1 {
		t.Fatalf("expected 1 remote-seen even on ATU, got %d", len(seens))
	}
	if seens[0].version != res.NewHead {
		t.Errorf("seen.version = %q, want %q", seens[0].version, res.NewHead)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Pull unhappy paths
// ─────────────────────────────────────────────────────────────────────

func TestService_Pull_EmptyPath_NoJournalCall(t *testing.T) {
	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Pull(PullOptions{Path: ""}); err == nil {
		t.Fatal("expected error for empty path")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("error path leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Pull_NonRepoPath_NoJournalCall(t *testing.T) {
	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Pull(PullOptions{Path: t.TempDir()}); err == nil {
		t.Fatal("expected error for non-repo path")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("non-repo error leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Pull_DirtyWorktree_NoJournalCall(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	// Advance bare so pull has something to do.
	advance := t.TempDir()
	pr, _ := gogit.PlainClone(advance, false, &gogit.CloneOptions{URL: bare})
	_ = os.WriteFile(filepath.Join(advance, "remote.txt"), []byte("r"), 0o644)
	pwt, _ := pr.Worktree()
	_, _ = pwt.Add("remote.txt")
	_, _ = pwt.Commit("rs", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	})
	_ = pr.Push(&gogit.PushOptions{RemoteName: "origin"})

	// Dirty the local worktree.
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Pull(PullOptions{Path: work}); err == nil {
		t.Fatal("expected dirty-worktree refusal")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("dirty refusal leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Pull_DetachedHEAD_NoJournalCall(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	wr, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	h, _ := wr.Head()
	if err := wr.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, h.Hash())); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Pull(PullOptions{Path: work}); err == nil {
		t.Fatal("expected detached-HEAD refusal")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("detached refusal leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

func TestService_Pull_AuthFailure_NoJournalCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("WWW-Authenticate", `Basic realm="Git"`)
		rw.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	work := pullAuthFixture(t, srv.URL+"/repo.git")

	svc, jrnl := newServiceWithJournal(t)
	if _, err := svc.Pull(PullOptions{Path: work, PAT: "bad"}); err == nil {
		t.Fatal("expected auth error")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("auth error leaked into journal: syncs=%+v seens=%+v", syncs, seens)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Edge: nil journal must never panic
// ─────────────────────────────────────────────────────────────────────

func TestService_NilJournal_PushIsSafe(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	// nil journal - the Service must not deref it.
	svc := NewService(NewManager(), nil, nil, nil)
	if _, err := svc.Push(PushOptions{Path: work}); err != nil {
		t.Fatalf("Push with nil journal: %v", err)
	}
}

func TestService_NilJournal_PullIsSafe(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewManager(), nil, nil, nil)
	if _, err := svc.Pull(PullOptions{Path: work}); err != nil {
		t.Fatalf("Pull with nil journal: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Service.PullWithStash - wiring tests
// ─────────────────────────────────────────────────────────────────────
//
// PullWithStash sits on top of Manager.PullWithStash; the Service's
// only added value is (a) reading journal Pending() to feed the
// snapshot manifest and (b) calling RecordRemoteSeen on success. So
// these tests focus on the journal-side contract.

// TestService_PullWithStash_ReadsPendingFromJournal - the Service's
// only PullWithStash-specific contract is that it reads pending paths
// from the journal and feeds them to the manager. Set pending to a
// path that's actually dirty; expect it to round-trip.
func TestService_PullWithStash_ReadsPendingFromJournal(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("user-edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Advance bare with an unrelated file so pull has something to do.
	advance := t.TempDir()
	pr, _ := gogit.PlainClone(advance, false, &gogit.CloneOptions{URL: bare})
	_ = os.WriteFile(filepath.Join(advance, "remote.txt"), []byte("r"), 0o644)
	pwt, _ := pr.Worktree()
	_, _ = pwt.Add("remote.txt")
	_, _ = pwt.Commit("rs", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	})
	_ = pr.Push(&gogit.PushOptions{RemoteName: "origin"})

	svc, jrnl := newServiceWithJournal(t)
	jrnl.setPending("git", []journal.PendingChange{
		{Path: "seed.txt", Op: "update"},
	})

	res, err := svc.PullWithStash(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("PullWithStash: %v", err)
	}
	if len(res.Overridden) != 0 {
		t.Errorf("unexpected overrides: %v", res.Overridden)
	}
	if len(res.Restored) != 1 {
		t.Errorf("expected one restore, got %v", res.Restored)
	}

	// User's edit must be back on disk.
	got, _ := os.ReadFile(filepath.Join(work, "seed.txt"))
	if string(got) != "user-edit" {
		t.Errorf("seed.txt = %q, want %q", string(got), "user-edit")
	}

	// Service must have recorded the remote-seen on the post-pull
	// version (NewHead).
	_, seens := jrnl.snapshot()
	if len(seens) != 1 {
		t.Fatalf("expected 1 remote-seen, got %d", len(seens))
	}
	if seens[0].version != res.Pull.NewHead {
		t.Errorf("seen.version = %q, want %q", seens[0].version, res.Pull.NewHead)
	}
}

// TestService_PullWithStash_NilJournal - service must not panic
// when no journal is wired. The pending set degrades to empty,
// effectively a normal pull.
func TestService_PullWithStash_NilJournal(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewManager(), nil, nil, nil)
	if _, err := svc.PullWithStash(PullOptions{Path: work}); err != nil {
		t.Fatalf("PullWithStash with nil journal: %v", err)
	}
}

// TestService_PullWithStash_OverrideRecordsRemoteSeen - when pull
// overrides the user's local change (non-meta.json or merge can't
// reconcile), the pull part of the operation still succeeded. The
// remote-seen call must fire so the journal cursor advances.
func TestService_PullWithStash_OverrideRecordsRemoteSeen(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("user-edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Advance bare modifying the same file.
	advance := t.TempDir()
	pr, _ := gogit.PlainClone(advance, false, &gogit.CloneOptions{URL: bare})
	_ = os.WriteFile(filepath.Join(advance, "seed.txt"), []byte("remote-edit"), 0o644)
	pwt, _ := pr.Worktree()
	_, _ = pwt.Add("seed.txt")
	_, _ = pwt.Commit("rs", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	})
	_ = pr.Push(&gogit.PushOptions{RemoteName: "origin"})

	svc, jrnl := newServiceWithJournal(t)
	jrnl.setPending("git", []journal.PendingChange{
		{Path: "seed.txt", Op: "update"},
	})

	res, err := svc.PullWithStash(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("PullWithStash: %v", err)
	}
	if len(res.Overridden) != 1 {
		t.Errorf("expected override, got %v", res.Overridden)
	}

	_, seens := jrnl.snapshot()
	if len(seens) != 1 {
		t.Errorf("expected remote-seen call after successful pull (even with override), got %v", seens)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Sysgit dispatch
// ─────────────────────────────────────────────────────────────────────

type fakeFlags struct{ selfCloned bool }

func (f *fakeFlags) GitSelfCloned() bool { return f.selfCloned }

// fakeRoot is the test RootReader: the Service resolves its working folder from
// here instead of a frontend-passed path, mirroring the real config resolver.
type fakeRoot struct{ path string }

func (f *fakeRoot) GetRemoteRootPath() (string, error) { return f.path, nil }

type fakeSysgit struct {
	available bool
	gotPath   string
	gotRemote string
	err       error
	calls     int
	upToDate  bool
}

func (f *fakeSysgit) Available() bool { return f.available }

func (f *fakeSysgit) Fetch(workdir, remote string) error {
	f.calls++
	f.gotPath = workdir
	f.gotRemote = remote
	return f.err
}

func (f *fakeSysgit) Push(workdir, remote string) (bool, error) {
	f.calls++
	f.gotPath = workdir
	f.gotRemote = remote
	return f.upToDate, f.err
}

func (f *fakeSysgit) Pull(workdir, remote string) (bool, error) {
	f.calls++
	f.gotPath = workdir
	f.gotRemote = remote
	return f.upToDate, f.err
}

func TestService_Fetch_DispatchesToSysgitWhenToggleOnAndAvailable(t *testing.T) {
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	if _, err := svc.Fetch(FetchOptions{Path: "/repo", Remote: "origin"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if sys.calls != 1 {
		t.Fatalf("sysgit.Fetch calls = %d, want 1", sys.calls)
	}
	if sys.gotPath != "/repo" || sys.gotRemote != "origin" {
		t.Fatalf("got (%q,%q), want (/repo,origin)", sys.gotPath, sys.gotRemote)
	}
}

func TestService_Fetch_FallsBackToGogitWhenToggleOff(t *testing.T) {
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: false}, sys)

	// Empty path triggers go-git's "path required" error - confirms we
	// went down the go-git path, not sysgit (which would have called the
	// fake and never errored).
	_, err := svc.Fetch(FetchOptions{Path: ""})
	if err == nil {
		t.Fatal("expected go-git path-required error")
	}
	if sys.calls != 0 {
		t.Errorf("sysgit invoked despite toggle off: %d calls", sys.calls)
	}
}

func TestService_Fetch_FallsBackToGogitWhenBinaryUnavailable(t *testing.T) {
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: false}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	_, err := svc.Fetch(FetchOptions{Path: ""})
	if err == nil {
		t.Fatal("expected go-git path-required error")
	}
	if sys.calls != 0 {
		t.Errorf("sysgit invoked despite Available=false: %d calls", sys.calls)
	}
}

func TestService_Fetch_SysgitErrorPropagates(t *testing.T) {
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true, err: errFakeAuth}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	_, err := svc.Fetch(FetchOptions{Path: "/repo"})
	if err != errFakeAuth {
		t.Fatalf("got %v, want errFakeAuth", err)
	}
}

var errFakeAuth = fakeErr("fake auth error")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

func TestService_Push_DispatchesToSysgit(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true, upToDate: false}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	res, err := svc.Push(PushOptions{Path: work, Remote: "origin"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if sys.calls != 1 || sys.gotPath != work {
		t.Fatalf("sysgit.Push not invoked correctly: calls=%d path=%q", sys.calls, sys.gotPath)
	}
	if res == nil || res.NewHead == "" {
		t.Fatalf("expected NewHead populated, got %+v", res)
	}
	syncs, _ := jrnl.snapshot()
	if len(syncs) != 1 {
		t.Fatalf("expected sync marker, got syncs=%v", syncs)
	}
	if syncs[0].version != res.NewHead {
		t.Fatalf("journal version %q != NewHead %q", syncs[0].version, res.NewHead)
	}
}

func TestService_Push_SysgitUpToDateRecordsRemoteSeen(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true, upToDate: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	res, err := svc.Push(PushOptions{Path: work, Remote: "origin"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !res.AlreadyUpToDate {
		t.Fatal("expected AlreadyUpToDate=true")
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 {
		t.Errorf("phantom sync marker on up-to-date push: %v", syncs)
	}
	if len(seens) != 1 {
		t.Errorf("expected remote-seen, got %v", seens)
	}
}

func TestService_Pull_DispatchesToSysgit(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	res, err := svc.Pull(PullOptions{Path: work, Remote: "origin"})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if sys.calls != 1 {
		t.Fatalf("sysgit.Pull calls = %d, want 1", sys.calls)
	}
	if res == nil || res.NewHead == "" {
		t.Fatalf("expected NewHead populated, got %+v", res)
	}
	_, seens := jrnl.snapshot()
	if len(seens) != 1 {
		t.Errorf("expected remote-seen call, got %v", seens)
	}
}

func TestService_Fetch_NilSysgitFallsBack(t *testing.T) {
	// useSysgit() must reject when sys is nil even if flags say "yes".
	svc, _ := newServiceWithJournal(t)
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, nil)
	if _, err := svc.Fetch(FetchOptions{Path: ""}); err == nil {
		t.Fatal("expected go-git path-required error")
	}
}

func TestService_Fetch_NilFlagsFallsBack(t *testing.T) {
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, nil, sys)
	if _, err := svc.Fetch(FetchOptions{Path: ""}); err == nil {
		t.Fatal("expected go-git path-required error")
	}
	if sys.calls != 0 {
		t.Errorf("sysgit invoked with nil flags: %d calls", sys.calls)
	}
}

func TestService_Pull_SysgitErrorSkipsJournal(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true, err: errFakeAuth}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	if _, err := svc.Pull(PullOptions{Path: work, Remote: "origin"}); err != errFakeAuth {
		t.Fatalf("got %v, want errFakeAuth", err)
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("error leaked into journal: syncs=%v seens=%v", syncs, seens)
	}
}

func TestService_Push_SysgitFallbackWhenUnavailable(t *testing.T) {
	// Toggle on but binary missing → must NOT call sysgit; instead
	// fall through to go-git which (with no path/remote/PAT) will
	// produce its usual error. The fake's call counter proves we
	// didn't shell out.
	svc, _ := newServiceWithJournal(t)
	sys := &fakeSysgit{available: false}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)
	_, _ = svc.Push(PushOptions{Path: ""})
	if sys.calls != 0 {
		t.Errorf("sysgit.Push invoked despite Available=false: %d calls", sys.calls)
	}
}

func TestService_Push_SysgitNilJournalIsSafe(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	// Service with nil journal - sysgit path must not panic on
	// post-op recording.
	svc := NewService(NewManager(), nil, nil, nil)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	if _, err := svc.Push(PushOptions{Path: work, Remote: "origin"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestService_Pull_SysgitNilJournalIsSafe(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc := NewService(NewManager(), nil, nil, nil)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	if _, err := svc.Pull(PullOptions{Path: work, Remote: "origin"}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

func TestService_Push_SysgitNonRepoPathLeavesJournalAlone(t *testing.T) {
	// sysgit "succeeded" (fake returns nil) but the path isn't a repo,
	// so headHash returns "". Journal must remain untouched - recording
	// an empty version would corrupt the cursor.
	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	res, err := svc.Push(PushOptions{Path: t.TempDir(), Remote: "origin"})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res.NewHead != "" {
		t.Errorf("expected empty NewHead on non-repo, got %q", res.NewHead)
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("journal touched with empty version: syncs=%v seens=%v", syncs, seens)
	}
}

func TestService_Push_SysgitErrorSkipsJournal(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, jrnl := newServiceWithJournal(t)
	sys := &fakeSysgit{available: true, err: errFakeAuth}
	AttachSysgit(svc, &fakeFlags{selfCloned: true}, sys)

	if _, err := svc.Push(PushOptions{Path: work, Remote: "origin"}); err != errFakeAuth {
		t.Fatalf("got %v, want errFakeAuth", err)
	}
	syncs, seens := jrnl.snapshot()
	if len(syncs) != 0 || len(seens) != 0 {
		t.Errorf("error leaked into journal: syncs=%v seens=%v", syncs, seens)
	}
}
