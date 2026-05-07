package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
)

// kvFS is the fs surface KV needs. *system.Manager satisfies it
// (see app.go); tests pass a small struct that talks to t.TempDir().
// Kept minimal so swapping backends is a one-line change later.
type kvFS interface {
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
}

// KV is the per-plugin key-value store. Each plugin's bag of
// values lives at <root>/<plugin-id>.json — one file per plugin
// so deleting a plugin is just removing its folder + this file.
//
// The JSON shape is `map[string]any` so plugin authors can store
// numbers, strings, bools, lists, and nested maps without thinking
// about a schema. Lua-side bindings round-trip values through
// the same lvalue↔Go converter the runtime uses.
//
// Reads and writes are serialized through one mutex. Realistic
// plugin workloads aren't write-heavy, and contention is hard to
// reason about anyway when scripts can call kv from arbitrary
// goroutines (hooks fan-in from save paths). One lock keeps the
// invariant trivial: the in-memory cache and the on-disk file
// never disagree.
type KV struct {
	fs   kvFS
	root string

	mu    sync.Mutex
	cache map[string]map[string]any // plugin-id → bag
}

// NewKV constructs an empty KV rooted at <root>. The directory is
// lazily created on first write — a fresh app with zero plugins
// using kv leaves no stray folders behind.
func NewKV(fs kvFS, root string) *KV {
	return &KV{fs: fs, root: root, cache: map[string]map[string]any{}}
}

// Get returns (value, true, nil) when the key exists, (nil, false, nil)
// when the plugin has nothing at that key, or (_, _, err) when the
// on-disk JSON is unparseable. Loads the plugin's bag on first
// access for that plugin id.
func (s *KV) Get(pluginID, key string) (any, bool, error) {
	if !validID(pluginID) {
		return nil, false, fmt.Errorf("%w: bad plugin id %q", ErrManifestInvalid, pluginID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bag, err := s.loadLocked(pluginID)
	if err != nil {
		return nil, false, err
	}
	v, ok := bag[key]
	return v, ok, nil
}

// Set writes a value for (pluginID, key) and persists the plugin's
// whole bag atomically. Concurrent Sets serialize on the mutex.
func (s *KV) Set(pluginID, key string, value any) error {
	if !validID(pluginID) {
		return fmt.Errorf("%w: bad plugin id %q", ErrManifestInvalid, pluginID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bag, err := s.loadLocked(pluginID)
	if err != nil {
		return err
	}
	bag[key] = value
	return s.saveLocked(pluginID, bag)
}

// Delete removes a key. Deleting a missing key is a silent no-op
// — same shape as `delete(map, key)` in Go.
func (s *KV) Delete(pluginID, key string) error {
	if !validID(pluginID) {
		return fmt.Errorf("%w: bad plugin id %q", ErrManifestInvalid, pluginID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bag, err := s.loadLocked(pluginID)
	if err != nil {
		return err
	}
	if _, ok := bag[key]; !ok {
		return nil
	}
	delete(bag, key)
	return s.saveLocked(pluginID, bag)
}

// Keys returns the plugin's keys, sorted ascending. Stable order
// makes Lua iteration predictable across runs.
func (s *KV) Keys(pluginID string) ([]string, error) {
	if !validID(pluginID) {
		return nil, fmt.Errorf("%w: bad plugin id %q", ErrManifestInvalid, pluginID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bag, err := s.loadLocked(pluginID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(bag))
	for k := range bag {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// loadLocked returns the plugin's bag, hydrating it from disk on
// first access. Caller must hold s.mu.
func (s *KV) loadLocked(pluginID string) (map[string]any, error) {
	if bag, ok := s.cache[pluginID]; ok {
		return bag, nil
	}
	path := s.pathFor(pluginID)
	if !s.fs.FileExists(path) {
		bag := map[string]any{}
		s.cache[pluginID] = bag
		return bag, nil
	}
	raw, err := s.fs.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("kv: load %s: %w", path, err)
	}
	var bag map[string]any
	if err := json.Unmarshal([]byte(raw), &bag); err != nil {
		return nil, fmt.Errorf("kv: parse %s: %w", path, err)
	}
	if bag == nil {
		bag = map[string]any{}
	}
	s.cache[pluginID] = bag
	return bag, nil
}

func (s *KV) saveLocked(pluginID string, bag map[string]any) error {
	if err := s.fs.EnsureDirectory(s.root); err != nil {
		return fmt.Errorf("kv: ensure dir: %w", err)
	}
	raw, err := json.MarshalIndent(bag, "", "  ")
	if err != nil {
		return fmt.Errorf("kv: marshal: %w", err)
	}
	if err := s.fs.SaveFile(s.pathFor(pluginID), string(raw)+"\n"); err != nil {
		return fmt.Errorf("kv: save: %w", err)
	}
	return nil
}

func (s *KV) pathFor(pluginID string) string {
	return filepath.Join(s.root, pluginID+".json")
}
