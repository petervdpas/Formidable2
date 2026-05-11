package nav

import (
	"errors"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type fakeTemplates struct {
	have map[string]*template.Template
	err  error
}

func (f *fakeTemplates) LoadTemplate(name string) (*template.Template, error) {
	if f.err != nil {
		return nil, f.err
	}
	tpl, ok := f.have[name]
	if !ok {
		return nil, nil
	}
	return tpl, nil
}

type fakeForms struct {
	have map[string]map[string]*storage.Form // template → datafile → form
}

func (f *fakeForms) LoadForm(t, d string) *storage.Form {
	if f.have == nil {
		return nil
	}
	return f.have[t][d]
}

type fakeConfig struct {
	mu      sync.Mutex
	updates []map[string]any
}

func (c *fakeConfig) UpdateUserConfig(p map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, p)
	return nil
}

type captureEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
}

type capturedEvent struct {
	name string
	data any
}

func (e *captureEmitter) Emit(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, capturedEvent{name, data})
}

type captureHistory struct {
	pushed []string
}

func (h *captureHistory) Push(href string) { h.pushed = append(h.pushed, href) }

func newTestManager() (*Manager, *fakeTemplates, *fakeForms, *fakeConfig, *captureEmitter) {
	tpls := &fakeTemplates{have: map[string]*template.Template{
		"basic.yaml": {Filename: "basic.yaml"},
	}}
	forms := &fakeForms{have: map[string]map[string]*storage.Form{
		"basic.yaml": {"sane.meta.json": {}},
	}}
	cfg := &fakeConfig{}
	emit := &captureEmitter{}
	m := NewManager(tpls, forms, cfg, emit, nil, nil)
	return m, tpls, forms, cfg, emit
}

func TestManager_NavigateToFormidable_Success(t *testing.T) {
	m, _, _, cfg, emit := newTestManager()
	res, err := m.NavigateToFormidable("formidable://basic.yaml:sane.meta.json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Success {
		t.Fatalf("not success: %+v", res)
	}
	if res.Target.Template != "basic.yaml" || res.Target.Datafile != "sane.meta.json" {
		t.Errorf("target = %+v", res.Target)
	}

	if len(cfg.updates) != 1 {
		t.Fatalf("expected one config update, got %d", len(cfg.updates))
	}
	u := cfg.updates[0]
	if u["selected_template"] != "basic.yaml" {
		t.Errorf("selected_template not set: %v", u)
	}
	if u["selected_data_file"] != "sane.meta.json" {
		t.Errorf("selected_data_file not set: %v", u)
	}
	if u["context_ribbon"] != "storage" {
		t.Errorf("context_ribbon not flipped to storage: %v", u)
	}

	if len(emit.events) != 1 || emit.events[0].name != EventChanged {
		t.Errorf("expected one nav:changed event, got %+v", emit.events)
	}
}

func TestManager_NavigateToFormidable_BadURL(t *testing.T) {
	m, _, _, cfg, emit := newTestManager()
	res, err := m.NavigateToFormidable("not-a-url")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Success {
		t.Errorf("should not succeed on bad url")
	}
	if res.Error == "" {
		t.Errorf("expected error message")
	}
	if len(cfg.updates) != 0 {
		t.Errorf("config should not be touched on bad url")
	}
	if len(emit.events) != 0 {
		t.Errorf("no event on bad url; got %+v", emit.events)
	}
}

func TestManager_NavigateToFormidable_MissingTemplate(t *testing.T) {
	m, _, _, cfg, _ := newTestManager()
	res, _ := m.NavigateToFormidable("formidable://does-not-exist.yaml:sane.meta.json")
	if res.Success {
		t.Errorf("missing template should fail")
	}
	if res.Target == nil {
		t.Errorf("Target should be reported even on validation failure")
	}
	if len(cfg.updates) != 0 {
		t.Errorf("config should not be touched")
	}
}

func TestManager_NavigateToFormidable_MissingDatafile(t *testing.T) {
	m, _, _, cfg, _ := newTestManager()
	res, _ := m.NavigateToFormidable("formidable://basic.yaml:missing.meta.json")
	if res.Success {
		t.Errorf("missing datafile should fail")
	}
	if len(cfg.updates) != 0 {
		t.Errorf("config should not be touched")
	}
}

func TestManager_NavigateToFormidable_TemplateLoadError(t *testing.T) {
	m, tpls, _, cfg, _ := newTestManager()
	tpls.err = errors.New("disk gone")
	res, _ := m.NavigateToFormidable("formidable://basic.yaml:sane.meta.json")
	if res.Success {
		t.Errorf("loader error should fail")
	}
	if len(cfg.updates) != 0 {
		t.Errorf("config should not be touched")
	}
}

func TestManager_NavigateToFormidable_PushesHistory(t *testing.T) {
	tpls := &fakeTemplates{have: map[string]*template.Template{
		"basic.yaml": {Filename: "basic.yaml"},
	}}
	forms := &fakeForms{have: map[string]map[string]*storage.Form{
		"basic.yaml": {"sane.meta.json": {}},
	}}
	cfg := &fakeConfig{}
	emit := &captureEmitter{}
	hist := &captureHistory{}
	m := NewManager(tpls, forms, cfg, emit, hist, nil)

	if _, err := m.NavigateToFormidable("formidable://basic.yaml:sane.meta.json"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	want := []string{"formidable://basic.yaml:sane.meta.json"}
	if len(hist.pushed) != 1 || hist.pushed[0] != want[0] {
		t.Fatalf("history.pushed=%v, want %v", hist.pushed, want)
	}
}

func TestManager_NavigateToFormidable_NoPushOnFailure(t *testing.T) {
	tpls := &fakeTemplates{have: map[string]*template.Template{
		"basic.yaml": {Filename: "basic.yaml"},
	}}
	forms := &fakeForms{have: map[string]map[string]*storage.Form{
		"basic.yaml": {"sane.meta.json": {}},
	}}
	cfg := &fakeConfig{}
	emit := &captureEmitter{}
	hist := &captureHistory{}
	m := NewManager(tpls, forms, cfg, emit, hist, nil)

	if _, err := m.NavigateToFormidable("formidable://basic.yaml:missing.meta.json"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if len(hist.pushed) != 0 {
		t.Fatalf("history.pushed=%v, want empty (form missing)", hist.pushed)
	}
}
