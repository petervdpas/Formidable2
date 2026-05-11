// Package history owns the back/forward navigation stack. It is a pure
// data structure over opaque href strings — no URL parsing, no nav,
// no config. The service layer composes it with nav.Manager (replay)
// and config persistence; this module just keeps the stack honest.
package history

import "sync"

const defaultMaxSize = 20

type State struct {
	CanBack    bool `json:"can_back"`
	CanForward bool `json:"can_forward"`
}

type Snapshot struct {
	Stack []string `json:"stack"`
	Index int      `json:"index"`
}

type Manager struct {
	mu               sync.Mutex
	stack            []string
	index            int
	maxSize          int
	suppressNextPush bool
}

func NewManager(maxSize int) *Manager {
	if maxSize < 1 {
		maxSize = defaultMaxSize
	}
	return &Manager{stack: nil, index: -1, maxSize: maxSize}
}

func (m *Manager) Push(href string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if href == "" {
		return
	}
	if m.suppressNextPush {
		m.suppressNextPush = false
		return
	}
	if m.index >= 0 && m.index < len(m.stack) && m.stack[m.index] == href {
		return
	}

	m.stack = append(m.stack[:m.index+1], href)
	if len(m.stack) > m.maxSize {
		drop := len(m.stack) - m.maxSize
		m.stack = m.stack[drop:]
	}
	m.index = len(m.stack) - 1
}

func (m *Manager) Back() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.index <= 0 {
		return "", false
	}
	m.index--
	return m.stack[m.index], true
}

func (m *Manager) Forward() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.index >= len(m.stack)-1 {
		return "", false
	}
	m.index++
	return m.stack[m.index], true
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return State{
		CanBack:    m.index > 0,
		CanForward: m.index >= 0 && m.index < len(m.stack)-1,
	}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.stack))
	copy(out, m.stack)
	return Snapshot{Stack: out, Index: m.index}
}

func (m *Manager) Restore(stack []string, index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	filtered := stack[:0:0]
	for _, h := range stack {
		if h != "" {
			filtered = append(filtered, h)
		}
	}
	if len(filtered) > m.maxSize {
		filtered = filtered[len(filtered)-m.maxSize:]
	}

	switch {
	case len(filtered) == 0:
		m.stack = nil
		m.index = -1
	case index < 0:
		m.stack = filtered
		m.index = -1
	case index >= len(filtered):
		m.stack = filtered
		m.index = len(filtered) - 1
	default:
		m.stack = filtered
		m.index = index
	}
}

func (m *Manager) SetSuppressNextPush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.suppressNextPush = true
}
