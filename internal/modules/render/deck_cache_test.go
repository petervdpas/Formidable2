package render

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// slideForm builds a form whose slide field holds one text block with the given
// text, so BuildDeck produces HTML that reflects the text.
func slideForm(text string) *storage.Form {
	return &storage.Form{Data: map[string]any{
		"slide": map[string]any{
			"blocks": []any{
				map[string]any{"kind": "text", "content": text, "x": 0, "y": 0, "w": 200, "h": 80},
			},
		},
	}}
}

func slideDeckManager(store *fakeFormStore) *Manager {
	tpl := &template.Template{Fields: []template.Field{{Key: "slide", Type: "slide"}}}
	return NewManager(&fakeTemplateLoader{tpl: tpl}, store, nil, nil, nil)
}

// With a rev source wired, BuildDeck reuses the cached HTML while the rev is
// unchanged, and rebuilds once the rev bumps.
func TestBuildDeck_CachesByRevision(t *testing.T) {
	store := &fakeFormStore{form: slideForm("ALPHA")}
	m := slideDeckManager(store)
	var rev int64 = 1
	m.SetRevFunc(func() (int64, error) { return rev, nil })

	first, err := m.BuildDeck("t.yaml", []string{"x.meta.json"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(first.HTML, "ALPHA") {
		t.Fatalf("first deck missing content; got %q", first.HTML)
	}

	// Underlying data changes but the rev does NOT: a cache hit serves the stale
	// (still-valid-for-this-rev) HTML.
	store.form = slideForm("BETA")
	cached, _ := m.BuildDeck("t.yaml", []string{"x.meta.json"})
	if cached.HTML != first.HTML {
		t.Errorf("expected cache hit at same rev; got a rebuild:\n%q", cached.HTML)
	}

	// Rev bumps (a write happened): the deck rebuilds and reflects the new data.
	rev = 2
	rebuilt, _ := m.BuildDeck("t.yaml", []string{"x.meta.json"})
	if !strings.Contains(rebuilt.HTML, "BETA") {
		t.Errorf("expected rebuild after rev bump to show new data; got %q", rebuilt.HTML)
	}
}

// A different datafile set is a different cache key (decks are not conflated).
func TestBuildDeck_CacheKeyIncludesDatafiles(t *testing.T) {
	store := &fakeFormStore{form: slideForm("SAME")}
	m := slideDeckManager(store)
	m.SetRevFunc(func() (int64, error) { return 1, nil })

	one, _ := m.BuildDeck("t.yaml", []string{"a.meta.json"})
	// One vs two slides: distinct keys, so the second is built (2 sections), not
	// served from the single-slide entry.
	two, _ := m.BuildDeck("t.yaml", []string{"a.meta.json", "b.meta.json"})
	if strings.Count(one.HTML, "<section") == strings.Count(two.HTML, "<section") {
		t.Errorf("distinct datafile sets should not share a cache entry: %d vs %d sections",
			strings.Count(one.HTML, "<section"), strings.Count(two.HTML, "<section"))
	}
}

// Without a rev source, BuildDeck always builds fresh (no caching).
func TestBuildDeck_NoRevFuncAlwaysFresh(t *testing.T) {
	store := &fakeFormStore{form: slideForm("ONE")}
	m := slideDeckManager(store)

	a, _ := m.BuildDeck("t.yaml", []string{"x.meta.json"})
	store.form = slideForm("TWO")
	b, _ := m.BuildDeck("t.yaml", []string{"x.meta.json"})
	if a.HTML == b.HTML {
		t.Error("without a rev source BuildDeck must not cache")
	}
	if !strings.Contains(b.HTML, "TWO") {
		t.Errorf("uncached build should reflect new data; got %q", b.HTML)
	}
}

func cacheLen(m *Manager) int {
	m.deckMu.Lock()
	defer m.deckMu.Unlock()
	return len(m.deckCache)
}

// Unhappy: a failed build must NOT be cached, and a later success at the SAME
// rev must still rebuild (no poisoned entry, no cached error).
func TestBuildDeck_BuildErrorNotCached(t *testing.T) {
	tpl := &template.Template{Fields: []template.Field{{Key: "slide", Type: "slide"}}}
	loader := &fakeTemplateLoader{tpl: tpl, err: errors.New("load boom")}
	store := &fakeFormStore{form: slideForm("OK")}
	m := NewManager(loader, store, nil, nil, nil)
	m.SetRevFunc(func() (int64, error) { return 1, nil })

	if _, err := m.BuildDeck("t.yaml", []string{"a.meta.json"}); err == nil {
		t.Fatal("expected build error")
	}
	if n := cacheLen(m); n != 0 {
		t.Errorf("errored build was cached: %d entries", n)
	}

	// Fix the loader; SAME rev. A cached error would wrongly resurface here.
	loader.err = nil
	d, err := m.BuildDeck("t.yaml", []string{"a.meta.json"})
	if err != nil {
		t.Fatalf("expected success after fix, got %v", err)
	}
	if !strings.Contains(d.HTML, "OK") {
		t.Errorf("post-fix build missing content; got %q", d.HTML)
	}
}

// Unhappy: when the rev source itself errors, BuildDeck can't establish the
// invalidation key, so it builds fresh every time and caches nothing.
func TestBuildDeck_RevFuncErrorBypassesCache(t *testing.T) {
	store := &fakeFormStore{form: slideForm("ONE")}
	m := slideDeckManager(store)
	m.SetRevFunc(func() (int64, error) { return 0, errors.New("rev unavailable") })

	a, _ := m.BuildDeck("t.yaml", []string{"a.meta.json"})
	store.form = slideForm("TWO")
	b, _ := m.BuildDeck("t.yaml", []string{"a.meta.json"})
	if a.HTML == b.HTML {
		t.Error("rev-source error must bypass the cache (build fresh each time)")
	}
	if !strings.Contains(b.HTML, "TWO") {
		t.Errorf("bypassed build should reflect new data; got %q", b.HTML)
	}
	if n := cacheLen(m); n != 0 {
		t.Errorf("rev-error path must not populate the cache: %d entries", n)
	}
}

// Unhappy edge: an empty datafile set is a valid, cacheable build (empty deck),
// not a crash.
func TestBuildDeck_EmptyDatafiles(t *testing.T) {
	m := slideDeckManager(&fakeFormStore{form: slideForm("X")})
	m.SetRevFunc(func() (int64, error) { return 1, nil })

	d, err := m.BuildDeck("t.yaml", nil)
	if err != nil {
		t.Fatalf("empty deck errored: %v", err)
	}
	if strings.Contains(d.HTML, "<section") {
		t.Errorf("empty datafiles should yield no sections; got %q", d.HTML)
	}
	if n := cacheLen(m); n != 1 {
		t.Errorf("empty deck should still cache one entry; got %d", n)
	}
}

// Concurrency: many simultaneous BuildDeck calls at the same rev must be
// race-free (run with -race) and converge to a single cache entry.
func TestBuildDeck_ConcurrentSameRev(t *testing.T) {
	m := slideDeckManager(&fakeFormStore{form: slideForm("C")})
	m.SetRevFunc(func() (int64, error) { return 7, nil })

	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.BuildDeck("t.yaml", []string{"a.meta.json"}); err != nil {
				t.Errorf("concurrent build: %v", err)
			}
		}()
	}
	wg.Wait()
	if n := cacheLen(m); n != 1 {
		t.Errorf("concurrent builds should converge to 1 entry; got %d", n)
	}
}
