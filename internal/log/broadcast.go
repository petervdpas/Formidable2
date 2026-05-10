package log

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Entry is a flattened slog.Record suitable for transport over Wails
// events / JSON. Attrs is a flat map (groups are joined into dotted
// keys) so the frontend can render without knowing slog internals.
type Entry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// EmitFunc fans an Entry out to whatever transport the host installs
// (typically Wails Events.Emit). Set may be called once after the
// host's event runtime is ready; pre-installation entries are buffered
// in the ring and replayed via Recent() on demand.
type EmitFunc func(e Entry)

// Broadcaster owns the ring buffer + emitter hook. It exposes a
// slog.Handler via Handler(); WithAttrs/WithGroup return derived
// handlers that share the same buffer but carry their own context.
type Broadcaster struct {
	mu   sync.RWMutex
	ring []Entry
	head int
	full bool
	cap  int
	emit EmitFunc
}

func NewBroadcaster(capacity int) *Broadcaster {
	if capacity <= 0 {
		capacity = 500
	}
	return &Broadcaster{
		ring: make([]Entry, capacity),
		cap:  capacity,
	}
}

// Handler returns a fresh slog.Handler bound to this broadcaster.
func (b *Broadcaster) Handler() slog.Handler { return &bcHandler{b: b} }

func (b *Broadcaster) push(e Entry) {
	b.mu.Lock()
	b.ring[b.head] = e
	b.head = (b.head + 1) % b.cap
	if b.head == 0 {
		b.full = true
	}
	emit := b.emit
	b.mu.Unlock()

	if emit != nil {
		emit(e)
	}
}

// SetEmitter installs (or clears) the per-record emit hook. Safe to
// call from any goroutine; existing in-flight Handle calls finish
// against whichever pointer they captured.
func (b *Broadcaster) SetEmitter(fn EmitFunc) {
	b.mu.Lock()
	b.emit = fn
	b.mu.Unlock()
}

// bcHandler is the slog.Handler view of a Broadcaster. WithAttrs/
// WithGroup safely clone this struct (no lock inside) while keeping
// the shared *Broadcaster pointer.
type bcHandler struct {
	b      *Broadcaster
	groups []string
	attrs  []slog.Attr
}

func (h *bcHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *bcHandler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{
		Time:  r.Time,
		Level: r.Level.String(),
		Msg:   r.Message,
	}
	if n := r.NumAttrs() + len(h.attrs); n > 0 {
		e.Attrs = make(map[string]any, n)
		for _, a := range h.attrs {
			collectAttr(e.Attrs, h.groups, a)
		}
		r.Attrs(func(a slog.Attr) bool {
			collectAttr(e.Attrs, h.groups, a)
			return true
		})
	}
	h.b.push(e)
	return nil
}

func (h *bcHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := *h
	cp.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &cp
}

func (h *bcHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	cp := *h
	cp.groups = append(append([]string{}, h.groups...), name)
	return &cp
}

// Recent returns up to n most-recent entries in chronological order.
// n<=0 returns everything currently in the ring.
func (b *Broadcaster) Recent(n int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	size := b.cap
	if !b.full {
		size = b.head
	}
	if n <= 0 || n > size {
		n = size
	}

	out := make([]Entry, 0, n)
	start := b.head - n
	if !b.full && start < 0 {
		start = 0
	}
	for i := 0; i < n; i++ {
		idx := ((start + i) % b.cap)
		if idx < 0 {
			idx += b.cap
		}
		out = append(out, b.ring[idx])
	}
	return out
}

func collectAttr(dst map[string]any, groups []string, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	v := a.Value.Resolve()
	key := a.Key
	if len(groups) > 0 {
		key = joinKey(groups, key)
	}
	if v.Kind() == slog.KindGroup {
		nestedGroups := append(groups, a.Key)
		for _, ga := range v.Group() {
			collectAttr(dst, nestedGroups, ga)
		}
		return
	}
	dst[key] = v.Any()
}

func joinKey(groups []string, key string) string {
	out := ""
	for i, g := range groups {
		if i > 0 {
			out += "."
		}
		out += g
	}
	if key != "" {
		if out != "" {
			out += "."
		}
		out += key
	}
	return out
}

// multiHandler fans Handle calls to a list of slog.Handlers. The
// Enabled check is the OR of all child Enabled calls.
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(hs ...slog.Handler) slog.Handler {
	return &multiHandler{handlers: hs}
}

func (m *multiHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		_ = h.Handle(ctx, r.Clone())
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		cp[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: cp}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	cp := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		cp[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: cp}
}
