package history

import "log/slog"

// Navigator replays a Back/Forward target.
type Navigator interface {
	NavigateToFormidable(href string) error
}

// EventEmitter broadcasts state events. Nil silences emit.
type EventEmitter interface {
	Emit(name string, data any)
}

// Persister writes the current stack snapshot back to user config.
type Persister interface {
	PersistSnapshot(s Snapshot)
}

// EventState is the Wails event name broadcast after each Back/Forward
// or Push. Payload is State.
const EventState = "history:state"

// Controller composes Manager with nav replay, an event emitter, and
// optional persistence. It is the composition-root-facing surface:
// nav calls Push, Vue calls (via Service) Back/Forward/State, and the
// composition root calls SetNavigator + Broadcast at boot.
type Controller struct {
	m         *Manager
	nav       Navigator
	emitter   EventEmitter
	persister Persister
	log       *slog.Logger
}

func NewController(m *Manager, nav Navigator, e EventEmitter, p Persister, log *slog.Logger) *Controller {
	if log == nil {
		log = slog.Default()
	}
	return &Controller{m: m, nav: nav, emitter: e, persister: p, log: log}
}

// SetNavigator wires the replay target after construction. Solves the
// chicken-and-egg between Controller (needs Navigator for Back/Forward)
// and nav.Manager (needs HistoryPusher = Controller). Boot-time only;
// not concurrency-safe.
func (c *Controller) SetNavigator(n Navigator) {
	c.nav = n
}

func (c *Controller) State() State {
	return c.m.State()
}

// Push is the nav-side entry point. It mutates the stack, persists if
// the composition root wired a Persister, and broadcasts state. Empty
// hrefs and duplicates-at-cursor short-circuit silently inside
// Manager - broadcast/persist fire only on real mutations, detected
// via a snapshot-equality check.
func (c *Controller) Push(href string) {
	before := c.m.Snapshot()
	c.m.Push(href)
	after := c.m.Snapshot()
	if snapsEqual(before, after) {
		return
	}
	c.persist(after)
	c.broadcast()
}

// Back moves the cursor one step, replays the resulting href through
// the navigator, and broadcasts the new state. At the start of the
// stack it is a no-op (returns the current state, no error).
func (c *Controller) Back() (State, error) {
	href, ok := c.m.Back()
	if !ok {
		return c.m.State(), nil
	}
	if err := c.replay(href); err != nil {
		return c.m.State(), err
	}
	c.persist(c.m.Snapshot())
	c.broadcast()
	return c.m.State(), nil
}

// Forward is the mirror of Back.
func (c *Controller) Forward() (State, error) {
	href, ok := c.m.Forward()
	if !ok {
		return c.m.State(), nil
	}
	if err := c.replay(href); err != nil {
		return c.m.State(), err
	}
	c.persist(c.m.Snapshot())
	c.broadcast()
	return c.m.State(), nil
}

// Broadcast pushes the current state to subscribers. Public so the
// composition root can fire it once on startup after Restore so the
// ribbon initializes correctly.
func (c *Controller) Broadcast() {
	c.broadcast()
}

func (c *Controller) replay(href string) error {
	c.m.SetSuppressNextPush()
	if c.nav == nil {
		return nil
	}
	return c.nav.NavigateToFormidable(href)
}

func (c *Controller) broadcast() {
	if c.emitter == nil {
		return
	}
	c.emitter.Emit(EventState, c.m.State())
}

func (c *Controller) persist(snap Snapshot) {
	if c.persister == nil {
		return
	}
	c.persister.PersistSnapshot(snap)
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

// Service is the Wails-bound facade. Only Back/Forward/State are
// exposed to the frontend; everything else (Push, SetNavigator,
// Broadcast) stays on Controller, off the wire.
type Service struct{ c *Controller }

func NewService(c *Controller) *Service { return &Service{c: c} }

func (s *Service) Back() (State, error) { return s.c.Back() }

func (s *Service) Forward() (State, error) { return s.c.Forward() }

func (s *Service) State() State { return s.c.State() }
