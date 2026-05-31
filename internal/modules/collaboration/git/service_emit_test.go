package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type recordingEmitter struct{ names []string }

func (r *recordingEmitter) Emit(name string, _ any) { r.names = append(r.names, name) }
func (r *recordingEmitter) has(n string) bool {
	for _, x := range r.names {
		if x == n {
			return true
		}
	}
	return false
}

// pushNewCommit advances the bare so a clone of it can pull something.
func pushNewCommit(t *testing.T, bare string) {
	t.Helper()
	tmp := t.TempDir()
	pr, err := gogit.PlainClone(tmp, false, &gogit.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "remote.txt"), []byte("r"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, _ := pr.Worktree()
	if _, err := wt.Add("remote.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("rs", &gogit.CommitOptions{
		Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := pr.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatal(err)
	}
}

// An advancing pull changed the working tree, so it must emit context:reloaded:
// the frontend reloads off this backend signal, not its own dispatch.
func TestService_Pull_EmitsContextReloadedOnAdvance(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	pushNewCommit(t, bare)

	svc, _ := newServiceWithJournal(t)
	em := &recordingEmitter{}
	AttachEmitter(svc, em)

	res, err := svc.Pull(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.AlreadyUpToDate {
		t.Fatal("expected an advancing pull")
	}
	if !em.has("context:reloaded") {
		t.Errorf("advancing pull must emit context:reloaded, got %v", em.names)
	}
}

// An up-to-date pull changed nothing, so it must NOT emit (no wasted reload).
func TestService_Pull_NoEmitWhenUpToDate(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}

	svc, _ := newServiceWithJournal(t)
	em := &recordingEmitter{}
	AttachEmitter(svc, em)

	if _, err := svc.Pull(PullOptions{Path: work}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if em.has("context:reloaded") {
		t.Errorf("up-to-date pull must not emit, got %v", em.names)
	}
}

// A clone brings a whole new working tree in, so it must emit context:reloaded.
func TestService_Clone_EmitsContextReloaded(t *testing.T) {
	bare := makeBareRepo(t)
	dest := filepath.Join(t.TempDir(), "clone")

	svc, _ := newServiceWithJournal(t)
	em := &recordingEmitter{}
	AttachEmitter(svc, em)

	if _, err := svc.Clone(CloneOptions{URL: bare, Dest: dest}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if !em.has("context:reloaded") {
		t.Errorf("Clone must emit context:reloaded, got %v", em.names)
	}
}

// Discard reverts the working tree, so it must emit context:reloaded.
func TestService_Discard_EmitsContextReloaded(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "seed.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, _ := newServiceWithJournal(t)
	em := &recordingEmitter{}
	AttachEmitter(svc, em)

	if err := svc.Discard(DiscardOptions{Path: work, File: "seed.txt"}); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if !em.has("context:reloaded") {
		t.Errorf("Discard must emit context:reloaded, got %v", em.names)
	}
}

// An advancing PullWithStash changed the tree, so it must emit.
func TestService_PullWithStash_EmitsOnAdvance(t *testing.T) {
	bare := makeBareRepo(t)
	work := t.TempDir()
	if _, err := gogit.PlainClone(work, false, &gogit.CloneOptions{URL: bare}); err != nil {
		t.Fatal(err)
	}
	pushNewCommit(t, bare)

	svc, _ := newServiceWithJournal(t)
	em := &recordingEmitter{}
	AttachEmitter(svc, em)

	res, err := svc.PullWithStash(PullOptions{Path: work})
	if err != nil {
		t.Fatalf("PullWithStash: %v", err)
	}
	if res.Pull == nil || res.Pull.AlreadyUpToDate {
		t.Fatal("expected an advancing pull")
	}
	if !em.has("context:reloaded") {
		t.Errorf("advancing PullWithStash must emit, got %v", em.names)
	}
}
