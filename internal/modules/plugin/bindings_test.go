package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────
// Test mocks — small, in-package implementations of the access
// interfaces bindings.go consumes. Real wiring (template.Manager,
// dataprovider.Manager, render.Manager, *system.Manager) lands in
// app.go; tests use these so the binding layer is unit-testable
// without hauling in every backend module.
// ─────────────────────────────────────────────────────────────────

type mockTemplate struct {
	all map[string]map[string]any
}

func (m *mockTemplate) ListTemplates() []map[string]any {
	out := make([]map[string]any, 0, len(m.all))
	for _, t := range m.all {
		out = append(out, t)
	}
	return out
}
func (m *mockTemplate) GetTemplate(filename string) (map[string]any, error) {
	t, ok := m.all[filename]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}

type mockCollection struct {
	rows map[string][]map[string]any
}

func (m *mockCollection) ListCollection(tpl string) ([]map[string]any, error) {
	rows, ok := m.rows[tpl]
	if !ok {
		return nil, errors.New("template not found")
	}
	return rows, nil
}

type mockForm struct {
	loaded map[string]map[string]any
	saved  map[string]map[string]any
}

func (m *mockForm) LoadForm(tpl, df string) (map[string]any, error) {
	if v, ok := m.loaded[tpl+"/"+df]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}
func (m *mockForm) SaveForm(_ context.Context, tpl, df string, data map[string]any) error {
	if m.saved == nil {
		m.saved = map[string]map[string]any{}
	}
	m.saved[tpl+"/"+df] = data
	return nil
}

type mockRender struct {
	md   map[string]string
	html map[string]string
}

func (m *mockRender) RenderMarkdown(tpl, df string) (string, error) {
	if v, ok := m.md[tpl+"/"+df]; ok {
		return v, nil
	}
	return "", errors.New("no markdown")
}
func (m *mockRender) RenderHTML(tpl, df string) (string, error) {
	if v, ok := m.html[tpl+"/"+df]; ok {
		return v, nil
	}
	return "", errors.New("no html")
}

// mockFM records parse/build invocations and returns canned values.
// Build is implemented inline (not a recording dummy) so round-trip
// tests can assert the YAML stringification path actually executes.
type mockFM struct {
	parseData map[string]any
	parseBody string
	parseErr  error
	builds    []fmBuildCall
}
type fmBuildCall struct {
	data map[string]any
	body string
}

func (m *mockFM) Parse(_ string) (map[string]any, string, error) {
	return m.parseData, m.parseBody, m.parseErr
}
func (m *mockFM) Build(data map[string]any, body string) string {
	m.builds = append(m.builds, fmBuildCall{data: data, body: body})
	if len(data) == 0 {
		return body
	}
	// Trivial deterministic serialiser for assertions — order-agnostic;
	// tests inspect the recorded fmBuildCall instead of the string shape.
	return "---\n<fm>\n---\n" + body
}

type mockExec struct {
	calls []execCall
	res   ExecResult
	err   error
}
type execCall struct {
	cmd  string
	args []string
	opts ExecOptions
}

func (m *mockExec) Exec(cmd string, args []string, opts ExecOptions) (ExecResult, error) {
	m.calls = append(m.calls, execCall{cmd, append([]string(nil), args...), opts})
	return m.res, m.err
}

// realFS is a tiny FSAccess implementation rooted at t.TempDir()
// so fs tests get genuine OS semantics (mkdir-recursive, list,
// exists). Production wiring uses a thin adapter over *system.Manager.
type realFS struct{}

func (realFS) Read(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}
func (realFS) Write(p, c string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(c), 0o644)
}
func (realFS) Mkdir(p string) error { return os.MkdirAll(p, 0o755) }
func (realFS) List(p string) ([]string, error) {
	es, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out, nil
}
func (realFS) Exists(p string) bool { _, err := os.Stat(p); return err == nil }

// run is a test helper that executes a Lua source with full
// runtime deps populated. The script's `run()` function is
// invoked with no arg and the result returned.
func run(t *testing.T, src string, deps scriptOpts) RunResult {
	t.Helper()
	deps.Source = src
	if deps.Fn == "" {
		deps.Fn = "run"
	}
	res, err := runScript(deps)
	if err != nil {
		t.Fatalf("run: %v\nlogs: %v", err, res.LogLines)
	}
	return res
}

func runErr(t *testing.T, src string, deps scriptOpts) error {
	t.Helper()
	deps.Source = src
	if deps.Fn == "" {
		deps.Fn = "run"
	}
	_, err := runScript(deps)
	return err
}

// ─────────────────────────────────────────────────────────────────
// formidable.kv — round-trip and isolation
// ─────────────────────────────────────────────────────────────────

func TestBindings_KV_RoundTrip(t *testing.T) {
	kv := NewKV(kvTestFS{}, filepath.Join(t.TempDir(), "kv"))
	res := run(t, `
		function run()
			formidable.kv.set("name", "Alice")
			formidable.kv.set("age", 30)
			return { name = formidable.kv.get("name"), age = formidable.kv.get("age") }
		end`,
		scriptOpts{PluginID: "demo", KV: kv})
	got := res.Value.(map[string]any)
	if got["name"] != "Alice" || got["age"] != float64(30) {
		t.Fatalf("got %v", got)
	}
}

func TestBindings_KV_KeysSorted(t *testing.T) {
	kv := NewKV(kvTestFS{}, filepath.Join(t.TempDir(), "kv"))
	res := run(t, `
		function run()
			formidable.kv.set("z", 1)
			formidable.kv.set("a", 1)
			formidable.kv.set("m", 1)
			return formidable.kv.keys()
		end`,
		scriptOpts{PluginID: "demo", KV: kv})
	got := res.Value.([]any)
	if got[0] != "a" || got[1] != "m" || got[2] != "z" {
		t.Fatalf("got %v", got)
	}
}

func TestBindings_KV_DeleteThenMissingGetReturnsNil(t *testing.T) {
	kv := NewKV(kvTestFS{}, filepath.Join(t.TempDir(), "kv"))
	res := run(t, `
		function run()
			formidable.kv.set("k", "v")
			formidable.kv.delete("k")
			return formidable.kv.get("k") == nil
		end`,
		scriptOpts{PluginID: "demo", KV: kv})
	if res.Value != true {
		t.Fatalf("expected true, got %v", res.Value)
	}
}

func TestBindings_KV_NotConfiguredErrors(t *testing.T) {
	err := runErr(t, `function run() formidable.kv.set("k", "v") end`,
		scriptOpts{PluginID: "demo"}) // no KV
	if err == nil || !strings.Contains(err.Error(), "kv: not configured") {
		t.Fatalf("want not-configured error, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.template / collection / form / render
// ─────────────────────────────────────────────────────────────────

func TestBindings_Template_List(t *testing.T) {
	tpl := &mockTemplate{
		all: map[string]map[string]any{
			"a.yaml": {"filename": "a.yaml", "name": "A"},
			"b.yaml": {"filename": "b.yaml", "name": "B"},
		},
	}
	res := run(t, `
		function run()
			local list = formidable.template.list()
			return #list
		end`,
		scriptOpts{Template: tpl})
	if res.Value != float64(2) {
		t.Fatalf("got %v", res.Value)
	}
}

func TestBindings_Template_Get(t *testing.T) {
	tpl := &mockTemplate{
		all: map[string]map[string]any{
			"a.yaml": {"filename": "a.yaml", "name": "Alpha"},
		},
	}
	res := run(t, `
		function run() return formidable.template.get("a.yaml").name end`,
		scriptOpts{Template: tpl})
	if res.Value != "Alpha" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestBindings_Template_GetMissingErrors(t *testing.T) {
	tpl := &mockTemplate{all: map[string]map[string]any{}}
	err := runErr(t, `function run() return formidable.template.get("ghost.yaml") end`,
		scriptOpts{Template: tpl})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestBindings_Collection_List(t *testing.T) {
	c := &mockCollection{rows: map[string][]map[string]any{
		"a.yaml": {{"guid": "g1"}, {"guid": "g2"}},
	}}
	res := run(t, `
		function run()
			local rows = formidable.collection.list("a.yaml")
			return rows[1].guid .. "," .. rows[2].guid
		end`,
		scriptOpts{Collection: c})
	if res.Value != "g1,g2" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestBindings_Form_LoadSave(t *testing.T) {
	f := &mockForm{
		loaded: map[string]map[string]any{
			"a.yaml/x.meta.json": {"name": "Alice"},
		},
	}
	res := run(t, `
		function run()
			local d = formidable.form.load("a.yaml", "x.meta.json")
			d.name = "Bob"
			formidable.form.save("a.yaml", "x.meta.json", d)
			return formidable.form.load("a.yaml", "x.meta.json").name
		end`,
		scriptOpts{Form: f})
	// load reads from `loaded`, save writes to `saved` — so the
	// second load returns the original. Verify save was observed.
	if res.Value != "Alice" {
		t.Fatalf("load: got %v", res.Value)
	}
	saved := f.saved["a.yaml/x.meta.json"]
	if saved == nil || saved["name"] != "Bob" {
		t.Fatalf("save not seen: %v", f.saved)
	}
}

func TestBindings_Render_Markdown(t *testing.T) {
	r := &mockRender{md: map[string]string{"a.yaml/x.meta.json": "# Title"}}
	res := run(t, `
		function run() return formidable.render.markdown("a.yaml", "x.meta.json") end`,
		scriptOpts{Render: r})
	if res.Value != "# Title" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestBindings_Render_HTML(t *testing.T) {
	r := &mockRender{html: map[string]string{"a.yaml/x.meta.json": "<h1>Title</h1>"}}
	res := run(t, `
		function run() return formidable.render.html("a.yaml", "x.meta.json") end`,
		scriptOpts{Render: r})
	if res.Value != "<h1>Title</h1>" {
		t.Fatalf("got %v", res.Value)
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.render.frontmatter / formidable.render.pluginBlock —
// composed render+parse. Render dep returns canned markdown; FM dep
// is the same mockFM. These pin the composition, not the underlying
// primitives (already covered above).
// ─────────────────────────────────────────────────────────────────

func TestBindings_Render_Frontmatter_Composes(t *testing.T) {
	r := &mockRender{md: map[string]string{"a.yaml/x.meta.json": "---\nignored\n---\n# Body"}}
	fm := &mockFM{
		parseData: map[string]any{"plugins": map[string]any{"wikiwonder": map[string]any{"title": "X"}}},
		parseBody: "# Body",
	}
	res := run(t, `
		function run()
			local data, body = formidable.render.frontmatter("a.yaml", "x.meta.json")
			return { title = data.plugins.wikiwonder.title, body = body }
		end`,
		scriptOpts{PluginID: "wikiwonder", Render: r, FM: fm})
	m, _ := res.Value.(map[string]any)
	if m["title"] != "X" || m["body"] != "# Body" {
		t.Fatalf("composed render.frontmatter wrong: %+v", m)
	}
}

func TestBindings_Render_PluginBlock_Composes(t *testing.T) {
	r := &mockRender{md: map[string]string{"a.yaml/x.meta.json": "---\nignored\n---\n# Body"}}
	fm := &mockFM{
		parseData: map[string]any{"plugins": map[string]any{
			"wikiwonder": map[string]any{"path": "foo", "title": "X"},
			"hugo":       map[string]any{"draft": true},
		}},
		parseBody: "# Body",
	}
	res := run(t, `
		function run()
			local block = formidable.render.pluginBlock("a.yaml", "x.meta.json")
			return { title = block.title, path = block.path }
		end`,
		scriptOpts{PluginID: "wikiwonder", Render: r, FM: fm})
	m, _ := res.Value.(map[string]any)
	if m["title"] != "X" || m["path"] != "foo" {
		t.Fatalf("composed render.pluginBlock wrong: %+v", m)
	}
}

func TestBindings_Render_PluginBlock_NilWhenAbsent(t *testing.T) {
	r := &mockRender{md: map[string]string{"a.yaml/x.meta.json": "# Body only"}}
	fm := &mockFM{parseData: nil, parseBody: "# Body only"}
	res := run(t, `
		function run()
			local block = formidable.render.pluginBlock("a.yaml", "x.meta.json")
			return { has_block = (block ~= nil) }
		end`,
		scriptOpts{PluginID: "wikiwonder", Render: r, FM: fm})
	m, _ := res.Value.(map[string]any)
	if got, _ := m["has_block"].(bool); got {
		t.Fatalf("expected nil block for no-FM input, got %v", m["has_block"])
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.fm — parse/build round-trip for plugin-side frontmatter
// manipulation. Real YAML semantics are tested in the render package;
// these tests pin the Lua binding glue (two return values for parse,
// nil-data shortcut for build).
// ─────────────────────────────────────────────────────────────────

func TestBindings_FM_Parse_NoFrontmatter(t *testing.T) {
	fm := &mockFM{parseData: nil, parseBody: "# Hello"}
	res := run(t, `
		function run()
			local data, body = formidable.fm.parse("# Hello")
			return { has_data = (data ~= nil), body = body }
		end`,
		scriptOpts{FM: fm})
	m, _ := res.Value.(map[string]any)
	if got, _ := m["has_data"].(bool); got {
		t.Fatalf("expected has_data=false, got %v", m["has_data"])
	}
	if got, _ := m["body"].(string); got != "# Hello" {
		t.Fatalf("body = %q, want %q", got, "# Hello")
	}
}

func TestBindings_FM_Parse_WithFrontmatter(t *testing.T) {
	fm := &mockFM{
		parseData: map[string]any{"title": "Hello", "wiki": map[string]any{"path": "p"}},
		parseBody: "# Body",
	}
	res := run(t, `
		function run()
			local data, body = formidable.fm.parse("---\ntitle: Hello\n---\n# Body")
			return {
				title = data.title,
				wiki_path = data.wiki.path,
				body = body,
			}
		end`,
		scriptOpts{FM: fm})
	m, _ := res.Value.(map[string]any)
	if m["title"] != "Hello" {
		t.Fatalf("title = %v", m["title"])
	}
	if m["wiki_path"] != "p" {
		t.Fatalf("wiki.path = %v", m["wiki_path"])
	}
	if m["body"] != "# Body" {
		t.Fatalf("body = %v", m["body"])
	}
}

func TestBindings_FM_Build_NilData_PassesThroughBody(t *testing.T) {
	fm := &mockFM{}
	res := run(t, `
		function run() return formidable.fm.build(nil, "raw body") end`,
		scriptOpts{FM: fm})
	if res.Value != "raw body" {
		t.Fatalf("got %v, want %q", res.Value, "raw body")
	}
	if len(fm.builds) != 1 || fm.builds[0].data != nil || fm.builds[0].body != "raw body" {
		t.Fatalf("unexpected builds: %+v", fm.builds)
	}
}

func TestBindings_FM_Build_TablePrependsFrontmatter(t *testing.T) {
	fm := &mockFM{}
	res := run(t, `
		function run()
			return formidable.fm.build({ title = "X", tags = { "a", "b" } }, "# Body")
		end`,
		scriptOpts{FM: fm})
	got, _ := res.Value.(string)
	if !strings.HasPrefix(got, "---\n") || !strings.Contains(got, "# Body") {
		t.Fatalf("build output looks wrong: %q", got)
	}
	if len(fm.builds) != 1 {
		t.Fatalf("expected 1 build call, got %d", len(fm.builds))
	}
	if fm.builds[0].data["title"] != "X" {
		t.Fatalf("data.title = %v", fm.builds[0].data["title"])
	}
}

func TestBindings_FM_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() return formidable.fm.parse("anything") end`,
		scriptOpts{}) // no FM dep
	if err == nil || !strings.Contains(err.Error(), "fm: not configured") {
		t.Fatalf("err = %v, want fm: not configured", err)
	}
}

func TestBindings_FM_PluginBlock_ReturnsOwnSlice(t *testing.T) {
	fm := &mockFM{
		parseData: map[string]any{
			"title": "Outer",
			"plugins": map[string]any{
				"wikiwonder": map[string]any{"path": "p", "title": "Inner"},
				"hugo":       map[string]any{"draft": false},
			},
		},
		parseBody: "# Body",
	}
	res := run(t, `
		function run()
			local data, _ = formidable.fm.parse("ignored")
			local block = formidable.fm.pluginBlock(data)
			return { title = block.title, path = block.path }
		end`,
		scriptOpts{PluginID: "wikiwonder", FM: fm})
	m, _ := res.Value.(map[string]any)
	if m["title"] != "Inner" || m["path"] != "p" {
		t.Fatalf("pluginBlock returned wrong slice: %+v", m)
	}
}

func TestBindings_FM_PluginBlock_NilWhenAbsent(t *testing.T) {
	fm := &mockFM{
		parseData: map[string]any{"title": "Outer"},
		parseBody: "# Body",
	}
	res := run(t, `
		function run()
			local data, _ = formidable.fm.parse("ignored")
			local block = formidable.fm.pluginBlock(data)
			return { has_block = (block ~= nil) }
		end`,
		scriptOpts{PluginID: "wikiwonder", FM: fm})
	m, _ := res.Value.(map[string]any)
	if got, _ := m["has_block"].(bool); got {
		t.Fatalf("expected nil block when no plugins.<id> present, got %v", m["has_block"])
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.progress — live tick events. Verified via a recording
// closure passed in via scriptOpts.ProgressOut; the runtime streams
// each call synchronously through the closure.
// ─────────────────────────────────────────────────────────────────

func TestBindings_Progress_Tick_StreamsThroughEmitter(t *testing.T) {
	var got []ProgressEvent
	emit := func(e ProgressEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.progress.tick(0, 3, "starting")
			formidable.progress.tick(1, 3, "did one")
			formidable.progress.tick(3, 3, "done")
		end`,
		scriptOpts{ProgressOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(got), got)
	}
	if got[1].Done != 1 || got[1].Total != 3 || got[1].Message != "did one" {
		t.Fatalf("event[1] = %+v", got[1])
	}
}

func TestBindings_Progress_Tick_OptionalArgs(t *testing.T) {
	var got []ProgressEvent
	emit := func(e ProgressEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.progress.tick()       -- all defaults
			formidable.progress.tick(5)      -- done only
			formidable.progress.tick(5, 10)  -- no message, no stage
		end`,
		scriptOpts{ProgressOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Done != 0 || got[0].Total != 0 || got[0].Message != "" || got[0].Stage != "" {
		t.Fatalf("event[0] = %+v", got[0])
	}
	if got[1].Done != 5 || got[1].Total != 0 {
		t.Fatalf("event[1] = %+v", got[1])
	}
	if got[2].Done != 5 || got[2].Total != 10 {
		t.Fatalf("event[2] = %+v", got[2])
	}
}

func TestBindings_Progress_Tick_StageArg(t *testing.T) {
	var got []ProgressEvent
	emit := func(e ProgressEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.progress.tick(1, 3, "item-a", "templates")
			formidable.progress.tick(2, 3, "item-b", "templates")
			formidable.progress.tick(3, 3, "first", "recepten")
		end`,
		scriptOpts{ProgressOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Stage != "templates" || got[0].Message != "item-a" {
		t.Fatalf("event[0] = %+v", got[0])
	}
	if got[2].Stage != "recepten" || got[2].Message != "first" {
		t.Fatalf("event[2] = %+v", got[2])
	}
}

func TestBindings_Progress_Tick_BackCompatNoStage(t *testing.T) {
	// 3-arg calls (no stage) still work — stage defaults to "" so
	// the dialog renders the bar without a stage header.
	var got []ProgressEvent
	emit := func(e ProgressEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.progress.tick(1, 2, "only-msg")
		end`,
		scriptOpts{ProgressOut: emit})
	if len(got) != 1 || got[0].Stage != "" || got[0].Message != "only-msg" {
		t.Fatalf("event = %+v", got)
	}
}

// TestBindings_Cancellation_AbortsRunningVM verifies L.SetContext
// plumbing: a cancelled context aborts the VM at the next
// instruction boundary and runScript surfaces ErrPluginCancelled.
// The Lua script spins; the test cancels via the run's ctx shortly
// after starting.
func TestBindings_Cancellation_AbortsRunningVM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel from a goroutine 50ms in — long enough for the VM to
	// enter the loop, short enough to keep the test fast.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	src := `
		function run()
			local i = 0
			while true do
				i = i + 1
				-- pure-Lua tight loop; SetContext aborts between
				-- instructions, so we never need to call into a
				-- formidable.* binding for cancel to take effect.
			end
		end`
	_, err := runScript(scriptOpts{Source: src, Fn: "run", Ctx: ctx})
	if !errors.Is(err, ErrPluginCancelled) {
		t.Fatalf("err = %v, want ErrPluginCancelled", err)
	}
}

func TestBindings_Cancellation_NilCtx_RunsNormally(t *testing.T) {
	res := run(t, `function run() return 42 end`, scriptOpts{})
	if v, _ := res.Value.(float64); v != 42 {
		t.Fatalf("got %v, want 42", res.Value)
	}
}

func TestBindings_Progress_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() formidable.progress.tick(1, 1, "x") end`,
		scriptOpts{}) // no ProgressOut
	if err == nil || !strings.Contains(err.Error(), "progress: not configured") {
		t.Fatalf("err = %v, want progress: not configured", err)
	}
}

func TestBindings_FM_PluginBlock_NilDataReturnsNil(t *testing.T) {
	fm := &mockFM{parseData: nil, parseBody: "# Body"}
	res := run(t, `
		function run()
			local data, _ = formidable.fm.parse("ignored")
			local block = formidable.fm.pluginBlock(data)
			return { has_block = (block ~= nil) }
		end`,
		scriptOpts{PluginID: "wikiwonder", FM: fm})
	m, _ := res.Value.(map[string]any)
	if got, _ := m["has_block"].(bool); got {
		t.Fatalf("expected nil block for nil data, got %v", m["has_block"])
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.fs — actual filesystem under t.TempDir()
// ─────────────────────────────────────────────────────────────────

func TestBindings_FS_WriteRead(t *testing.T) {
	tmp := t.TempDir()
	res := run(t, `
		function run()
			formidable.fs.write("`+filepath.Join(tmp, "x.txt")+`", "hello")
			return formidable.fs.read("`+filepath.Join(tmp, "x.txt")+`")
		end`,
		scriptOpts{FS: realFS{}})
	if res.Value != "hello" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestBindings_FS_MkdirListExists(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "deep", "nested")
	res := run(t, `
		function run()
			formidable.fs.mkdir("`+dir+`")
			formidable.fs.write("`+filepath.Join(dir, "a.txt")+`", "1")
			formidable.fs.write("`+filepath.Join(dir, "b.txt")+`", "2")
			return {
				exists = formidable.fs.exists("`+dir+`"),
				count  = #formidable.fs.list("`+dir+`")
			}
		end`,
		scriptOpts{FS: realFS{}})
	got := res.Value.(map[string]any)
	if got["exists"] != true || got["count"] != float64(2) {
		t.Fatalf("got %v", got)
	}
}

func TestBindings_FS_ReadMissingErrors(t *testing.T) {
	err := runErr(t, `function run() return formidable.fs.read("/no/such/file") end`,
		scriptOpts{FS: realFS{}})
	if err == nil {
		t.Fatal("want error")
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.exec — uses mockExec to verify args + opts threading
// ─────────────────────────────────────────────────────────────────

func TestBindings_Exec_HappyWithOpts(t *testing.T) {
	m := &mockExec{res: ExecResult{Stdout: "out", Stderr: "", Exit: 0}}
	res := run(t, `
		function run()
			local r = formidable.exec("git", {"status", "--short"}, { cwd = "/tmp" })
			return { exit = r.exit, stdout = r.stdout }
		end`,
		scriptOpts{Exec: m})
	got := res.Value.(map[string]any)
	if got["exit"] != float64(0) || got["stdout"] != "out" {
		t.Fatalf("got %v", got)
	}
	if len(m.calls) != 1 {
		t.Fatalf("calls: %v", m.calls)
	}
	c := m.calls[0]
	if c.cmd != "git" || !reflect.DeepEqual(c.args, []string{"status", "--short"}) {
		t.Fatalf("got cmd=%q args=%v", c.cmd, c.args)
	}
	if c.opts.Cwd != "/tmp" {
		t.Fatalf("cwd: %q", c.opts.Cwd)
	}
}

func TestBindings_Exec_TimeoutThreaded(t *testing.T) {
	m := &mockExec{res: ExecResult{}}
	_ = run(t, `
		function run()
			formidable.exec("ls", {}, { timeout_ms = 5000 })
		end`,
		scriptOpts{Exec: m})
	if got := m.calls[0].opts.Timeout; got != 5*time.Second {
		t.Fatalf("timeout: %v", got)
	}
}

func TestBindings_Exec_NoOptsAllowed(t *testing.T) {
	m := &mockExec{res: ExecResult{Stdout: "ok"}}
	_ = run(t, `
		function run() formidable.exec("ls", {}) end`,
		scriptOpts{Exec: m})
	if len(m.calls) != 1 {
		t.Fatalf("calls: %v", m.calls)
	}
}

// ─────────────────────────────────────────────────────────────────
// Nil-safe wrappers — every namespace errors clearly when its
// access dependency wasn't injected.
// ─────────────────────────────────────────────────────────────────

func TestBindings_NilDepsErrorClearly(t *testing.T) {
	cases := map[string]string{
		"template.list":    `function run() return formidable.template.list() end`,
		"template.get":     `function run() return formidable.template.get("x.yaml") end`,
		"collection.list":  `function run() return formidable.collection.list("x.yaml") end`,
		"form.load":        `function run() return formidable.form.load("x.yaml", "y.json") end`,
		"form.save":        `function run() return formidable.form.save("x.yaml", "y.json", {}) end`,
		"render.markdown":  `function run() return formidable.render.markdown("x.yaml", "y") end`,
		"render.html":      `function run() return formidable.render.html("x.yaml", "y") end`,
		"fs.read":          `function run() return formidable.fs.read("/x") end`,
		"exec":             `function run() return formidable.exec("ls", {}) end`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			err := runErr(t, src, scriptOpts{})
			if err == nil || !strings.Contains(err.Error(), "not configured") {
				t.Fatalf("want not-configured, got %v", err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.plugin — runtime self-introspection
// ─────────────────────────────────────────────────────────────────

func TestBindings_Plugin_FieldsAvailable(t *testing.T) {
	res := run(t, `
		function run()
			return {
				id      = formidable.plugin.id,
				name    = formidable.plugin.name,
				version = formidable.plugin.version,
				author  = formidable.plugin.author,
				desc    = formidable.plugin.description,
				mode    = formidable.plugin.mode,
				cmd     = formidable.plugin.command,
				server  = formidable.plugin.requires_internal_server,
				debug   = formidable.plugin.debug,
			}
		end
	`, scriptOpts{
		Plugin: PluginInfo{
			ID:                     "test-plugin",
			Name:                   "Test Plugin",
			Version:                "0.1.0",
			Author:                 "Peter",
			Description:            "This is a test",
			Mode:                   "form",
			Command:                "start",
			RequiresInternalServer: true,
			Debug:                  true,
		},
	})
	got, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", res.Value)
	}
	want := map[string]any{
		"id":      "test-plugin",
		"name":    "Test Plugin",
		"version": "0.1.0",
		"author":  "Peter",
		"desc":    "This is a test",
		"mode":    "form",
		"cmd":     "start",
		"server":  true,
		"debug":   true,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("plugin.%s: got %v, want %v", k, got[k], v)
		}
	}
}

func TestBindings_Plugin_FormFields(t *testing.T) {
	res := run(t, `
		function run()
			local out = {}
			for i, f in ipairs(formidable.plugin.form) do
				out[i] = (f.label or f.key) .. ":" .. f.type
			end
			return out
		end
	`, scriptOpts{
		Plugin: PluginInfo{
			Form: []map[string]any{
				{"key": "what", "type": "text", "label": "What?"},
				{"key": "input", "type": "file-path", "label": "Input"},
			},
		},
	})
	got, ok := res.Value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", res.Value)
	}
	if len(got) != 2 || got[0] != "What?:text" || got[1] != "Input:file-path" {
		t.Fatalf("unexpected form payload: %v", got)
	}
}

func TestBindings_Plugin_FormEmptyWhenAbsent(t *testing.T) {
	// No Form supplied — Lua sees an empty table, not nil. Means
	// `for _, f in ipairs(formidable.plugin.form)` is always safe,
	// no nil-checks needed in plugin code.
	res := run(t, `
		function run()
			local n = 0
			for _ in ipairs(formidable.plugin.form) do n = n + 1 end
			return n
		end
	`, scriptOpts{})
	if res.Value != float64(0) && res.Value != 0 {
		t.Fatalf("expected 0, got %v", res.Value)
	}
}

func TestBindings_Plugin_ZeroValuesWhenUnset(t *testing.T) {
	// No PluginInfo passed — everything reads as zero/empty without
	// raising an error so plugin authors can sniff for fields without
	// nil-checking.
	res := run(t, `
		function run()
			return {
				id   = formidable.plugin.id,
				mode = formidable.plugin.mode,
				srv  = formidable.plugin.requires_internal_server,
			}
		end
	`, scriptOpts{})
	got := res.Value.(map[string]any)
	if got["id"] != "" || got["mode"] != "" {
		t.Fatalf("expected empty strings, got %v", got)
	}
	if got["srv"] != false {
		t.Fatalf("expected false, got %v", got["srv"])
	}
}
