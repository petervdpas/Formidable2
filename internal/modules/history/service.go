package history

import "log/slog"

// Navigator is what the service needs from nav to replay a Back/Forward
// target. The composition root wraps nav.Manager.NavigateToFormidable
// (which returns *Result, error) into this thinner shape so history
// doesn't have to import nav and create a cycle (nav imports history
// to push on successful navigation).
type Navigator interface {
	NavigateToFormidable(href string) error
}

// EventEmitter mirrors nav.EventEmitter — composition root injects a
// Wails-backed implementation; nil is allowed and silences emit.
type EventEmitter interface {
	Emit(name string, data any)
}

// Persister writes the current stack snapshot back to user config when
// history.persist is on. Composition root supplies a shim that reads
// the live config flag and no-ops when persist is off — Service is
// kept oblivious to the setting.
type Persister interface {
	PersistSnapshot(s Snapshot)
}

// EventState is the Wails event name broadcast after each Back/Forward
// or Push. Payload is State.
const EventState = "history:state"

type Service struct {
	m         *Manager
	nav       Navigator
	emitter   EventEmitter
	persister Persister
	log       *slog.Logger
}

func NewService(m *Manager, nav Navigator, e EventEmitter, p Persister, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{m: m, nav: nav, emitter: e, persister: p, log: log}
}

// SetNavigator wires the replay target after construction. Solves the
// chicken-and-egg between history.Service (needs Navigator for Back/
// Forward) and nav.Manager (needs HistoryPusher = history.Service).
// Boot-time only; not concurrency-safe.
func (s *Service) SetNavigator(n Navigator) {
	s.nav = n
}

func (s *Service) State() State {
	return s.m.State()
}

// Push is the nav-side entry point. It mutates the stack, persists if
// the composition root wired a Persister, and broadcasts state. Empty
// hrefs and duplicates-at-cursor short-circuit silently inside
// Manager — we still broadcast/persist on real mutations only, which
// we detect via a snapshot-equality check.
func (s *Service) Push(href string) {
	before := s.m.Snapshot()
	s.m.Push(href)
	after := s.m.Snapshot()
	if snapsEqual(before, after) {
		return
	}
	s.persist(after)
	s.broadcast()
}

// Back moves the cursor one step, replays the resulting href through
// the navigator, and broadcasts the new state. At the start of the
// stack it is a no-op (returns the current state, no error).
func (s *Service) Back() (State, error) {
	href, ok := s.m.Back()
	if !ok {
		return s.m.State(), nil
	}
	if err := s.replay(href); err != nil {
		return s.m.State(), err
	}
	s.persist(s.m.Snapshot())
	s.broadcast()
	return s.m.State(), nil
}

// Forward is the mirror of Back.
func (s *Service) Forward() (State, error) {
	href, ok := s.m.Forward()
	if !ok {
		return s.m.State(), nil
	}
	if err := s.replay(href); err != nil {
		return s.m.State(), err
	}
	s.persist(s.m.Snapshot())
	s.broadcast()
	return s.m.State(), nil
}

// Broadcast pushes the current state to subscribers. Public so the
// composition root can fire it once on startup after Restore so the
// ribbon initializes correctly.
func (s *Service) Broadcast() {
	s.broadcast()
}

func (s *Service) replay(href string) error {
	s.m.SetSuppressNextPush()
	if s.nav == nil {
		return nil
	}
	return s.nav.NavigateToFormidable(href)
}

func (s *Service) broadcast() {
	if s.emitter == nil {
		return
	}
	s.emitter.Emit(EventState, s.m.State())
}

func (s *Service) persist(snap Snapshot) {
	if s.persister == nil {
		return
	}
	s.persister.PersistSnapshot(snap)
}

func snapsEqual(a, b Snapshot) bool {
	if a.Index != b.Index || len(a.Stack) != len(b.Stack) {
		return false
	}
	for i := range a.Stack {
		if a.Stack[i] != b.Stack[i] {
			return false
		}
	}
	return true
}
