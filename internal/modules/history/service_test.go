package history

import (
	"errors"
	"reflect"
	"testing"
)

type fakeNav struct {
	hrefs   []string
	err     error
	onCall  func(m *Manager)
	manager *Manager
}

func (f *fakeNav) NavigateToFormidable(href string) error {
	f.hrefs = append(f.hrefs, href)
	if f.onCall != nil {
		f.onCall(f.manager)
	}
	return f.err
}

type fakeEmitter struct {
	events []struct {
		name string
		data any
	}
}

func (f *fakeEmitter) Emit(name string, data any) {
	f.events = append(f.events, struct {
		name string
		data any
	}{name, data})
}

type fakePersister struct {
	snaps []Snapshot
}

func (f *fakePersister) PersistSnapshot(s Snapshot) {
	f.snaps = append(f.snaps, s)
}

func setupService(t *testing.T) (*Manager, *fakeNav, *fakeEmitter, *Service) {
	t.Helper()
	m := NewManager(20)
	nav := &fakeNav{manager: m}
	em := &fakeEmitter{}
	s := NewService(m, nav, em, nil, nil)
	return m, nav, em, s
}

func setupServiceWithPersister(t *testing.T) (*Manager, *fakeNav, *fakeEmitter, *fakePersister, *Service) {
	t.Helper()
	m := NewManager(20)
	nav := &fakeNav{manager: m}
	em := &fakeEmitter{}
	p := &fakePersister{}
	s := NewService(m, nav, em, p, nil)
	return m, nav, em, p, s
}

func TestService_State_Passthrough(t *testing.T) {
	m, _, _, s := setupService(t)
	m.Push("a")
	m.Push("b")

	if got := s.State(); !got.CanBack || got.CanForward {
		t.Fatalf("State=%+v, want canBack=true canForward=false", got)
	}
}

func TestService_Back_EmptyStack_NoNavCall(t *testing.T) {
	_, nav, em, s := setupService(t)

	st, err := s.Back()
	if err != nil {
		t.Fatalf("Back: %v", err)
	}
	if st.CanBack || st.CanForward {
		t.Fatalf("State after empty Back=%+v, want both false", st)
	}
	if len(nav.hrefs) != 0 {
		t.Fatalf("navigator called on empty stack: %v", nav.hrefs)
	}
	if len(em.events) != 0 {
		t.Fatalf("emitter fired on empty Back: %+v", em.events)
	}
}

func TestService_Back_DispatchesAndBroadcasts(t *testing.T) {
	m, nav, em, s := setupService(t)
	m.Push("a")
	m.Push("b")
	m.Push("c")

	st, err := s.Back()
	if err != nil {
		t.Fatalf("Back: %v", err)
	}
	if !reflect.DeepEqual(nav.hrefs, []string{"b"}) {
		t.Fatalf("nav.hrefs=%v, want [b]", nav.hrefs)
	}
	if !st.CanBack || !st.CanForward {
		t.Fatalf("state=%+v, want both true", st)
	}
	if len(em.events) != 1 || em.events[0].name != EventState {
		t.Fatalf("emitter events=%+v, want one %q", em.events, EventState)
	}
	if got, ok := em.events[0].data.(State); !ok || !got.CanBack || !got.CanForward {
		t.Fatalf("emitted payload=%+v, want State{true,true}", em.events[0].data)
	}
}

func TestService_Back_SuppressesReplayPush(t *testing.T) {
	m, nav, _, s := setupService(t)
	m.Push("a")
	m.Push("b")

	nav.onCall = func(mm *Manager) {
		mm.Push("c")
	}

	if _, err := s.Back(); err != nil {
		t.Fatalf("Back: %v", err)
	}

	snap := m.Snapshot()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(snap.Stack, want) || snap.Index != 0 {
		t.Fatalf("snapshot=%+v, want stack=%v index=0", snap, want)
	}
}

func TestService_Forward_DispatchesAndBroadcasts(t *testing.T) {
	m, nav, em, s := setupService(t)
	m.Push("a")
	m.Push("b")
	m.Push("c")
	if _, ok := m.Back(); !ok {
		t.Fatal("Back: ok=false")
	}
	if _, ok := m.Back(); !ok {
		t.Fatal("Back: ok=false")
	}

	if _, err := s.Forward(); err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if !reflect.DeepEqual(nav.hrefs, []string{"b"}) {
		t.Fatalf("nav.hrefs=%v, want [b]", nav.hrefs)
	}
	if len(em.events) != 1 || em.events[0].name != EventState {
		t.Fatalf("emitter events=%+v", em.events)
	}
}

func TestService_NavErrorPropagates(t *testing.T) {
	m, nav, em, s := setupService(t)
	m.Push("a")
	m.Push("b")
	nav.err = errors.New("template gone")

	st, err := s.Back()
	if err == nil || err.Error() != "template gone" {
		t.Fatalf("err=%v, want template gone", err)
	}
	if !st.CanForward {
		t.Fatalf("state=%+v, want canForward=true (index moved despite nav error)", st)
	}
	if len(em.events) != 0 {
		t.Fatalf("emitter fired on nav failure: %+v", em.events)
	}
}

func TestService_Broadcast_StandalonePush(t *testing.T) {
	m, _, em, s := setupService(t)
	m.Push("a")

	s.Broadcast()

	if len(em.events) != 1 || em.events[0].name != EventState {
		t.Fatalf("events=%+v, want one %q", em.events, EventState)
	}
	if got, ok := em.events[0].data.(State); !ok || got.CanBack || got.CanForward {
		t.Fatalf("payload=%+v, want State{false,false}", em.events[0].data)
	}
}

func TestService_NilNavigator_Tolerant(t *testing.T) {
	m := NewManager(20)
	em := &fakeEmitter{}
	s := NewService(m, nil, em, nil, nil)
	m.Push("a")
	m.Push("b")

	if _, err := s.Back(); err != nil {
		t.Fatalf("Back: %v", err)
	}
	if len(em.events) != 1 {
		t.Fatalf("events=%+v, want one (broadcast still fires)", em.events)
	}
}

func TestService_NilEmitter_Tolerant(t *testing.T) {
	m := NewManager(20)
	nav := &fakeNav{manager: m}
	s := NewService(m, nav, nil, nil, nil)
	m.Push("a")
	m.Push("b")

	if _, err := s.Back(); err != nil {
		t.Fatalf("Back: %v", err)
	}
	s.Broadcast()
}

func TestService_Push_DelegatesAndBroadcasts(t *testing.T) {
	m, _, em, p, s := setupServiceWithPersister(t)

	s.Push("a")
	s.Push("b")

	snap := m.Snapshot()
	if len(snap.Stack) != 2 || snap.Stack[1] != "b" {
		t.Fatalf("Push didn't reach manager: %+v", snap)
	}
	if len(em.events) != 2 {
		t.Fatalf("events=%+v, want 2 broadcasts", em.events)
	}
	if len(p.snaps) != 2 || p.snaps[1].Index != 1 {
		t.Fatalf("persist snaps=%+v", p.snaps)
	}
}

func TestService_Push_EmptyHrefSkipsBroadcastAndPersist(t *testing.T) {
	_, _, em, p, s := setupServiceWithPersister(t)

	s.Push("")

	if len(em.events) != 0 || len(p.snaps) != 0 {
		t.Fatalf("empty Push fired side-effects: events=%+v snaps=%+v", em.events, p.snaps)
	}
}

func TestService_Back_PersistsAfterReplay(t *testing.T) {
	m, _, _, p, s := setupServiceWithPersister(t)
	m.Push("a")
	m.Push("b")

	if _, err := s.Back(); err != nil {
		t.Fatalf("Back: %v", err)
	}
	if len(p.snaps) != 1 || p.snaps[0].Index != 0 {
		t.Fatalf("persist after Back: %+v", p.snaps)
	}
}

func TestService_NavError_SkipsPersist(t *testing.T) {
	m, nav, _, p, s := setupServiceWithPersister(t)
	m.Push("a")
	m.Push("b")
	nav.err = errors.New("gone")

	if _, err := s.Back(); err == nil {
		t.Fatal("expected nav error")
	}
	if len(p.snaps) != 0 {
		t.Fatalf("persist fired on nav error: %+v", p.snaps)
	}
}

func TestService_NilPersister_Tolerant(t *testing.T) {
	m := NewManager(20)
	em := &fakeEmitter{}
	s := NewService(m, nil, em, nil, nil)
	s.Push("a")
	s.Push("b")
	if _, err := s.Back(); err != nil {
		t.Fatalf("Back: %v", err)
	}
}
