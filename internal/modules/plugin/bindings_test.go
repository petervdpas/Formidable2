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
// Test mocks - small, in-package implementations of the access
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
	// Trivial deterministic serialiser for assertions - order-agnostic;
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
func (realFS) Copy(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}
func (realFS) Remove(p string) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

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
// formidable.kv - round-trip and isolation
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
	// load reads from `loaded`, save writes to `saved` - so the
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
// formidable.render.frontmatter / formidable.render.pluginBlock -
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
// formidable.fm - parse/build round-trip for plugin-side frontmatter
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
// formidable.run - two emitters drive the bar and statusmessage
// widgets. Verified via recording closures passed in via
// scriptOpts.RunBarOut / scriptOpts.RunStatOut.
// ─────────────────────────────────────────────────────────────────

func TestBindings_Run_Bar_StreamsThroughEmitter(t *testing.T) {
	var got []RunBarEvent
	emit := func(e RunBarEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.run.bar(0, 3)
			formidable.run.bar(1, 3)
			formidable.run.bar(3, 3)
		end`,
		scriptOpts{RunBarOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d bar events, want 3: %+v", len(got), got)
	}
	if got[1].Done != 1 || got[1].Total != 3 {
		t.Fatalf("event[1] = %+v", got[1])
	}
}

func TestBindings_Run_Chart_StreamsSpecThroughEmitter(t *testing.T) {
	var got []RunChartEvent
	emit := func(e RunChartEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.run.chart({ type = "bar", title = "T", result = { total = 7 } })
		end`,
		scriptOpts{RunChartOut: emit})
	if len(got) != 1 {
		t.Fatalf("got %d chart events, want 1: %+v", len(got), got)
	}
	spec := got[0].Spec
	if spec["type"] != "bar" || spec["title"] != "T" {
		t.Fatalf("spec = %+v", spec)
	}
	res, ok := spec["result"].(map[string]any)
	if !ok || res["total"] != float64(7) {
		t.Fatalf("spec.result = %+v", spec["result"])
	}
}

func TestBindings_Run_Bar_OptionalArgs(t *testing.T) {
	var got []RunBarEvent
	emit := func(e RunBarEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.run.bar()       -- both defaults
			formidable.run.bar(5)      -- done only
			formidable.run.bar(5, 10)
		end`,
		scriptOpts{RunBarOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Done != 0 || got[0].Total != 0 {
		t.Fatalf("event[0] = %+v", got[0])
	}
	if got[1].Done != 5 || got[1].Total != 0 {
		t.Fatalf("event[1] = %+v", got[1])
	}
	if got[2].Done != 5 || got[2].Total != 10 {
		t.Fatalf("event[2] = %+v", got[2])
	}
}

func TestBindings_Run_Status_StreamsThroughEmitter(t *testing.T) {
	var got []RunStatusEvent
	emit := func(e RunStatusEvent) { got = append(got, e) }
	run(t, `
		function run()
			formidable.run.status("starting")
			formidable.run.status("CH.02.md")
			formidable.run.status("done")
		end`,
		scriptOpts{RunStatOut: emit})
	if len(got) != 3 {
		t.Fatalf("got %d status events, want 3: %+v", len(got), got)
	}
	if got[1].Text != "CH.02.md" {
		t.Fatalf("event[1] = %+v", got[1])
	}
}

func TestBindings_Run_Status_EmptyDefault(t *testing.T) {
	var got []RunStatusEvent
	emit := func(e RunStatusEvent) { got = append(got, e) }
	run(t, `function run() formidable.run.status() end`,
		scriptOpts{RunStatOut: emit})
	if len(got) != 1 || got[0].Text != "" {
		t.Fatalf("events = %+v, want [{Text:\"\"}]", got)
	}
}

// TestBindings_Cancellation_AbortsRunningVM verifies L.SetContext
// plumbing: a cancelled context aborts the VM at the next
// instruction boundary and runScript surfaces ErrPluginCancelled.
// The Lua script spins; the test cancels via the run's ctx shortly
// after starting.
func TestBindings_Cancellation_AbortsRunningVM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel from a goroutine 50ms in - long enough for the VM to
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

// TestBindings_Cancellation_PollPredicate verifies the cheap
// predicate path: a long-running pcall-heavy loop can poll
// formidable.cancelled() at the top of each iteration and break
// out cleanly. The host signals cancel mid-loop via the ctx.
func TestBindings_Cancellation_PollPredicate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	src := `
		function run()
			local count = 0
			for i = 1, 100000 do
				if formidable.cancelled() then break end
				count = count + 1
				-- simulate "useful work" - a tiny busy spin so the
				-- loop doesn't blow through 100k iterations before
				-- the test's cancel signal arrives.
				for j = 1, 1000 do end
			end
			return count
		end`
	res, err := runScript(scriptOpts{Source: src, Fn: "run", Ctx: ctx})
	// Either the script poll noticed the cancel and broke cleanly
	// (CallByParam err = nil, post-call ctx check surfaces
	// ErrPluginCancelled) or gopher-lua's own ctx check fired
	// between bytecodes. Both paths must end at ErrPluginCancelled.
	if !errors.Is(err, ErrPluginCancelled) {
		t.Fatalf("err = %v, want ErrPluginCancelled (count=%v)", err, res.Value)
	}
}

// TestBindings_Cancellation_PostCallCheckCatchesPcallSwallow
// verifies the safety net: a script that swallows the context
// error via pcall would otherwise "complete normally", but the
// post-call ctx.Err() check still surfaces ErrPluginCancelled.
func TestBindings_Cancellation_PostCallCheckCatchesPcallSwallow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before the script even starts so the very first
	// instruction is post-cancel-context - the swallow is
	// guaranteed by pcall'ing a sleep-equivalent that errors on
	// the cancelled VM.
	cancel()
	src := `
		function run()
			-- pcall a no-op that returns immediately on a
			-- cancelled VM - the error (if any) is swallowed.
			pcall(function() return 1 end)
			return "completed"
		end`
	_, err := runScript(scriptOpts{Source: src, Fn: "run", Ctx: ctx})
	if !errors.Is(err, ErrPluginCancelled) {
		t.Fatalf("err = %v, want ErrPluginCancelled (post-call check missed cancellation)", err)
	}
}

func TestBindings_Cancelled_NilCtxReturnsFalse(t *testing.T) {
	res := run(t, `function run() return formidable.cancelled() end`, scriptOpts{})
	if got, _ := res.Value.(bool); got {
		t.Fatalf("formidable.cancelled() with nil ctx = %v, want false", res.Value)
	}
}

func TestBindings_Run_Bar_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() formidable.run.bar(1, 1) end`,
		scriptOpts{}) // no RunBarOut
	if err == nil || !strings.Contains(err.Error(), "run: not configured") {
		t.Fatalf("err = %v, want run: not configured", err)
	}
}

func TestBindings_Run_Status_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() formidable.run.status("x") end`,
		scriptOpts{}) // no RunStatOut
	if err == nil || !strings.Contains(err.Error(), "run: not configured") {
		t.Fatalf("err = %v, want run: not configured", err)
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
// formidable.fs - actual filesystem under t.TempDir()
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
// formidable.path - join + stripExt utilities (no host deps)
// ─────────────────────────────────────────────────────────────────

func TestBindings_Path_Join(t *testing.T) {
	res := run(t, `
		function run()
			return {
				simple = formidable.path.join("/a", "b"),
				trailing = formidable.path.join("/a/", "b"),
				multi = formidable.path.join("/a", "b", "c"),
				empty = formidable.path.join("/a", "", "c"),
				rel = formidable.path.join("a", "b"),
			}
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	for k, want := range map[string]string{
		"simple":   "/a/b",
		"trailing": "/a/b",
		"multi":    "/a/b/c",
		"empty":    "/a/c",
		"rel":      "a/b",
	} {
		if got[k] != want {
			t.Errorf("%s: got %q, want %q", k, got[k], want)
		}
	}
}

func TestBindings_Path_StripExt(t *testing.T) {
	res := run(t, `
		function run()
			return {
				yaml = formidable.path.stripExt("foo.yaml", ".yaml"),
				meta = formidable.path.stripExt("foo.meta.json", ".meta.json"),
				absent = formidable.path.stripExt("foo", ".yaml"),
				partial = formidable.path.stripExt("foo.json", ".meta.json"),
			}
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	for k, want := range map[string]string{
		"yaml":    "foo",
		"meta":    "foo",
		"absent":  "foo",
		"partial": "foo.json",
	} {
		if got[k] != want {
			t.Errorf("%s: got %q, want %q", k, got[k], want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.url - encode + decode (matches net/url PathEscape)
// ─────────────────────────────────────────────────────────────────

func TestBindings_URL_Encode_Decode_RoundTrip(t *testing.T) {
	res := run(t, `
		function run()
			local enc = formidable.url.encode("cover art (final).png")
			local dec = formidable.url.decode(enc)
			return { enc = enc, dec = dec }
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	if got["dec"] != "cover art (final).png" {
		t.Errorf("decode round-trip: got %q", got["dec"])
	}
	if !strings.Contains(got["enc"].(string), "%20") {
		t.Errorf("encode should percent-escape spaces: got %q", got["enc"])
	}
}

func TestBindings_URL_Decode_InvalidRaises(t *testing.T) {
	err := runErr(t,
		`function run() return formidable.url.decode("%XY") end`,
		scriptOpts{})
	if err == nil || !strings.Contains(err.Error(), "url.decode") {
		t.Fatalf("want url.decode error, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.rewrite.* - Lua-side post-render markdown rewriters
// shipped via builtins.lua. Composes formidable.path.stripExt, so a
// pure-Lua test through runScript covers both the embed + the
// rewriter itself.
// ─────────────────────────────────────────────────────────────────

func TestBindings_Rewrite_Markdown_Images(t *testing.T) {
	res := run(t, `
		function run()
			local md = "![alt](/api/images/recipes/cover%20art.png)"
			local out, images = formidable.rewrite.markdown(md, {
				template_stem = "recipes",
				image_path_prefix = ".images/",
			})
			local names = {}
			for n, _ in pairs(images) do table.insert(names, n) end
			return { md = out, image = names[1], count = #names }
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	if got["md"] != "![alt](.images/cover%20art.png)" {
		t.Errorf("md: got %q", got["md"])
	}
	if got["image"] != "cover%20art.png" {
		t.Errorf("image: got %q", got["image"])
	}
	if got["count"] != float64(1) {
		t.Errorf("count: got %v", got["count"])
	}
}

func TestBindings_Rewrite_Markdown_LabelledLink(t *testing.T) {
	res := run(t, `
		function run()
			local out = formidable.rewrite.markdown(
				"see [intro](formidable://recipes.yaml:overview.meta.json#top) here",
				{ link_path_format = "/{tpl}/{data}{hash}" })
			return out
		end`, scriptOpts{})
	want := "see [intro](/recipes/overview#top) here"
	if res.Value != want {
		t.Errorf("got %q, want %q", res.Value, want)
	}
}

func TestBindings_Rewrite_Markdown_BareLink(t *testing.T) {
	res := run(t, `
		function run()
			local out = formidable.rewrite.markdown(
				"see formidable://controls.yaml:CH.02.meta.json#sec end",
				{ link_path_format = "/{tpl}/{data}{hash}" })
			return out
		end`, scriptOpts{})
	want := "see [controls/CH.02](/controls/CH.02#sec) end"
	if res.Value != want {
		t.Errorf("got %q, want %q", res.Value, want)
	}
}

func TestBindings_Rewrite_Markdown_Combined(t *testing.T) {
	res := run(t, `
		function run()
			local md = "![](/api/images/r/a.png)\nsee [x](formidable://r.yaml:b.meta.json)"
			local out, images = formidable.rewrite.markdown(md, {
				template_stem      = "r",
				image_path_prefix  = ".images/",
				link_path_format   = "/{tpl}/{data}{hash}",
			})
			local has_image = false
			for n, _ in pairs(images) do
				if n == "a.png" then has_image = true end
			end
			return { md = out, has_image = has_image }
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	want := "![](.images/a.png)\nsee [x](/r/b)"
	if got["md"] != want {
		t.Errorf("md: got %q, want %q", got["md"], want)
	}
	if got["has_image"] != true {
		t.Errorf("image not collected")
	}
}

func TestBindings_Rewrite_Markdown_EmptyOptsIsNoOp(t *testing.T) {
	res := run(t, `
		function run()
			local md = "![](/api/images/r/a.png) [x](formidable://r.yaml:b.meta.json)"
			local out = formidable.rewrite.markdown(md, {})
			return out == md
		end`, scriptOpts{})
	if res.Value != true {
		t.Fatalf("empty opts should leave markdown untouched")
	}
}

func TestBindings_Rewrite_Markdown_OnlyImagesNoLinks(t *testing.T) {
	res := run(t, `
		function run()
			local md = "![](/api/images/r/a.png) [x](formidable://r.yaml:b.meta.json)"
			local out = formidable.rewrite.markdown(md, {
				template_stem = "r",
				image_path_prefix = ".images/",
			})
			return out
		end`, scriptOpts{})
	want := "![](.images/a.png) [x](formidable://r.yaml:b.meta.json)"
	if res.Value != want {
		t.Errorf("links should be untouched without link_path_format: got %q", res.Value)
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.fs.copy / formidable.fs.remove
// ─────────────────────────────────────────────────────────────────

func TestBindings_FS_Copy_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	from := filepath.Join(tmp, "src.txt")
	to := filepath.Join(tmp, "sub", "dst.txt")
	if err := os.WriteFile(from, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, `
		function run()
			formidable.fs.copy("`+from+`", "`+to+`")
		end`, scriptOpts{FS: realFS{}})
	got, err := os.ReadFile(to)
	if err != nil || string(got) != "hello" {
		t.Fatalf("copy: got %q err %v", got, err)
	}
}

func TestBindings_FS_Copy_MissingSourceErrors(t *testing.T) {
	err := runErr(t,
		`function run() formidable.fs.copy("/no/such", "/tmp/dst") end`,
		scriptOpts{FS: realFS{}})
	if err == nil || !strings.Contains(err.Error(), "fs.copy") {
		t.Fatalf("want fs.copy error, got %v", err)
	}
}

func TestBindings_FS_Remove_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "x.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, `function run() formidable.fs.remove("`+f+`") end`,
		scriptOpts{FS: realFS{}})
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: err=%v", err)
	}
}

func TestBindings_FS_Remove_MissingIsNoOp(t *testing.T) {
	run(t, `function run() formidable.fs.remove("/no/such/missing") end`,
		scriptOpts{FS: realFS{}})
}

// ─────────────────────────────────────────────────────────────────
// formidable.storage - image bytes lookup for wiki-export plugins
// ─────────────────────────────────────────────────────────────────

type mockStorage struct {
	images map[string][]byte
	err    error
}

func (m *mockStorage) ImageBytes(tpl, name string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if b, ok := m.images[tpl+"/"+name]; ok {
		return b, nil
	}
	return nil, nil
}

func TestBindings_Storage_ImageBytes_RoundTrip(t *testing.T) {
	m := &mockStorage{images: map[string][]byte{
		"recipes.yaml/cover.png": []byte("\x89PNG\r\n\x1a\nDATA"),
	}}
	res := run(t, `
		function run()
			return formidable.storage.imageBytes("recipes.yaml", "cover.png")
		end`,
		scriptOpts{Storage: m})
	got, ok := res.Value.(string)
	if !ok {
		t.Fatalf("want string, got %T (%v)", res.Value, res.Value)
	}
	if got != "\x89PNG\r\n\x1a\nDATA" {
		t.Fatalf("bytes round-trip: got %q", got)
	}
}

func TestBindings_Storage_ImageBytes_MissingReturnsNil(t *testing.T) {
	res := run(t, `
		function run()
			local b = formidable.storage.imageBytes("recipes.yaml", "ghost.png")
			return b == nil
		end`,
		scriptOpts{Storage: &mockStorage{}})
	if res.Value != true {
		t.Fatalf("want nil for missing image, got %v", res.Value)
	}
}

func TestBindings_Storage_ImageBytes_ErrorRaises(t *testing.T) {
	m := &mockStorage{err: errors.New("disk gone")}
	err := runErr(t, `
		function run() return formidable.storage.imageBytes("x.yaml", "y.png") end`,
		scriptOpts{Storage: m})
	if err == nil || !strings.Contains(err.Error(), "storage.imageBytes") {
		t.Fatalf("want storage.imageBytes error, got %v", err)
	}
}

func TestBindings_Storage_NotConfigured_Errors(t *testing.T) {
	err := runErr(t,
		`function run() return formidable.storage.imageBytes("x.yaml", "y.png") end`,
		scriptOpts{})
	if err == nil || !strings.Contains(err.Error(), "storage: not configured") {
		t.Fatalf("want storage: not configured, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// formidable.exec - uses mockExec to verify args + opts threading
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
// Nil-safe wrappers - every namespace errors clearly when its
// access dependency wasn't injected.
// ─────────────────────────────────────────────────────────────────

func TestBindings_NilDepsErrorClearly(t *testing.T) {
	cases := map[string]string{
		"template.list":   `function run() return formidable.template.list() end`,
		"template.get":    `function run() return formidable.template.get("x.yaml") end`,
		"collection.list": `function run() return formidable.collection.list("x.yaml") end`,
		"form.load":       `function run() return formidable.form.load("x.yaml", "y.json") end`,
		"form.save":       `function run() return formidable.form.save("x.yaml", "y.json", {}) end`,
		"render.markdown": `function run() return formidable.render.markdown("x.yaml", "y") end`,
		"render.html":     `function run() return formidable.render.html("x.yaml", "y") end`,
		"fs.read":         `function run() return formidable.fs.read("/x") end`,
		"exec":            `function run() return formidable.exec("ls", {}) end`,
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
// formidable.plugin - runtime self-introspection
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
	// No Form supplied - Lua sees an empty table, not nil. Means
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
	// No PluginInfo passed - everything reads as zero/empty without
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

// ─────────────────────────────────────────────────────────────────
// formidable.stats / formidable.facets - chart-neutral statistics
// ─────────────────────────────────────────────────────────────────

type mockStats struct {
	gotCol    *int
	gotPct    *float64
	gotPeriod string
}

func (m *mockStats) Distribution(_, _ string, col *int) (map[string]any, error) {
	m.gotCol = col
	return map[string]any{"kind": "distribution", "categories": []any{"a", "b"}, "total": 5}, nil
}
func (m *mockStats) NumericStats(_, _ string, col *int, pct *float64) (map[string]any, error) {
	m.gotCol, m.gotPct = col, pct
	return map[string]any{"kind": "scalar_stats", "scalars": map[string]any{"sum": 60.0}}, nil
}
func (m *mockStats) TimeSeries(_, _ string, col *int, period string) (map[string]any, error) {
	m.gotCol, m.gotPeriod = col, period
	return map[string]any{"kind": "timeseries", "categories": []any{"2026-01"}}, nil
}

type mockFacets struct {
	gotA, gotB string
}

func (m *mockFacets) FacetDistribution(_, key string) (map[string]any, error) {
	m.gotA = key
	return map[string]any{"kind": "distribution", "categories": []any{"high", "low"}}, nil
}
func (m *mockFacets) FacetCross(_, a, b string) (map[string]any, error) {
	m.gotA, m.gotB = a, b
	return map[string]any{"kind": "crosstab"}, nil
}
func (m *mockFacets) TotalForms(string) (int, error) { return 7, nil }

func TestBindings_Stats_Distribution(t *testing.T) {
	res := run(t, `
		function run()
			local r = formidable.stats.distribution("t", "status")
			return { kind = r.kind, first = r.categories[1], total = r.total }
		end`,
		scriptOpts{Stats: &mockStats{}})
	got := res.Value.(map[string]any)
	if got["kind"] != "distribution" || got["first"] != "a" || got["total"] != float64(5) {
		t.Fatalf("got %v", got)
	}
}

func TestBindings_Stats_Numeric_PassesColAndPercentile(t *testing.T) {
	ms := &mockStats{}
	run(t, `
		function run() return formidable.stats.numeric("t", "amount", 1, 90) end`,
		scriptOpts{Stats: ms})
	if ms.gotCol == nil || *ms.gotCol != 1 {
		t.Errorf("col = %v, want 1", ms.gotCol)
	}
	if ms.gotPct == nil || *ms.gotPct != 90 {
		t.Errorf("pct = %v, want 90", ms.gotPct)
	}
}

func TestBindings_Stats_Numeric_ScalarFieldOmitsCol(t *testing.T) {
	ms := &mockStats{}
	run(t, `function run() return formidable.stats.numeric("t", "amount") end`,
		scriptOpts{Stats: ms})
	if ms.gotCol != nil {
		t.Errorf("col = %v, want nil for scalar field", *ms.gotCol)
	}
	if ms.gotPct != nil {
		t.Errorf("pct = %v, want nil when omitted", *ms.gotPct)
	}
}

func TestBindings_Stats_TimeSeries_ArgOrder(t *testing.T) {
	// timeSeries(tpl, field, period [, col]) - period is arg 3, col arg 4.
	ms := &mockStats{}
	run(t, `function run() return formidable.stats.timeSeries("t", "due", "month", 2) end`,
		scriptOpts{Stats: ms})
	if ms.gotPeriod != "month" {
		t.Errorf("period = %q, want month", ms.gotPeriod)
	}
	if ms.gotCol == nil || *ms.gotCol != 2 {
		t.Errorf("col = %v, want 2", ms.gotCol)
	}
}

func TestBindings_Stats_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() return formidable.stats.distribution("t", "f") end`, scriptOpts{})
	if err == nil {
		t.Fatal("expected 'stats: not configured' error")
	}
}

func TestBindings_Facets_DistributionAndCross(t *testing.T) {
	mf := &mockFacets{}
	res := run(t, `
		function run()
			local d = formidable.facets.distribution("t", "prio")
			formidable.facets.cross("t", "prio", "stage")
			return { kind = d.kind, total = formidable.facets.totalForms("t") }
		end`,
		scriptOpts{Facets: mf})
	got := res.Value.(map[string]any)
	if got["kind"] != "distribution" || got["total"] != float64(7) {
		t.Fatalf("got %v", got)
	}
	if mf.gotA != "prio" || mf.gotB != "stage" {
		t.Errorf("cross args = (%q,%q), want (prio,stage)", mf.gotA, mf.gotB)
	}
}

func TestBindings_Facets_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() return formidable.facets.totalForms("t") end`, scriptOpts{})
	if err == nil {
		t.Fatal("expected 'facets: not configured' error")
	}
}

type mockStatObject struct {
	gotTpl, gotName string
	gotListTpl      string
}

func (m *mockStatObject) EvaluateObject(tpl, name string) (map[string]any, error) {
	m.gotTpl, m.gotName = tpl, name
	return map[string]any{"total": 5, "measures": []any{"count"}}, nil
}

func (m *mockStatObject) ListObjects(tpl string) ([]map[string]any, error) {
	m.gotListTpl = tpl
	return []map[string]any{
		{"name": "by-status", "label": "By status", "dsl": `count() by F["status"]`},
		{"name": "raw", "label": "", "dsl": "count()"},
	}, nil
}

func TestBindings_Statistical_EvaluatesNamedObject(t *testing.T) {
	ms := &mockStatObject{}
	got := run(t,
		`function run() local g = formidable.statistical("demo.yaml", "by-status"); return { total = g.total } end`,
		scriptOpts{StatObject: ms})
	m, ok := got.Value.(map[string]any)
	if !ok || m["total"] != float64(5) {
		t.Fatalf("return = %v, want total 5", got.Value)
	}
	if ms.gotTpl != "demo.yaml" || ms.gotName != "by-status" {
		t.Errorf("args = (%q, %q), want (demo.yaml, by-status)", ms.gotTpl, ms.gotName)
	}
}

func TestBindings_Statistical_ListReturnsCatalog(t *testing.T) {
	ms := &mockStatObject{}
	got := run(t,
		`function run()
			local objs = formidable.statistical.list("demo.yaml")
			return { n = #objs, first = objs[1].name, label = objs[1].label, dsl = objs[1].dsl }
		end`,
		scriptOpts{StatObject: ms})
	m, ok := got.Value.(map[string]any)
	if !ok || m["n"] != float64(2) || m["first"] != "by-status" ||
		m["label"] != "By status" || m["dsl"] != `count() by F["status"]` {
		t.Fatalf("return = %v", got.Value)
	}
	if ms.gotListTpl != "demo.yaml" {
		t.Errorf("list arg = %q, want demo.yaml", ms.gotListTpl)
	}
}

func TestBindings_Statistical_EvalMethodMatchesCallable(t *testing.T) {
	ms := &mockStatObject{}
	got := run(t,
		`function run() local g = formidable.statistical.eval("demo.yaml", "by-status"); return { total = g.total } end`,
		scriptOpts{StatObject: ms})
	m, ok := got.Value.(map[string]any)
	if !ok || m["total"] != float64(5) {
		t.Fatalf("return = %v, want total 5", got.Value)
	}
	if ms.gotTpl != "demo.yaml" || ms.gotName != "by-status" {
		t.Errorf("args = (%q, %q), want (demo.yaml, by-status)", ms.gotTpl, ms.gotName)
	}
}

func TestBindings_Statistical_NotConfigured_Errors(t *testing.T) {
	err := runErr(t, `function run() return formidable.statistical("t", "n") end`, scriptOpts{})
	if err == nil {
		t.Fatal("expected 'statistical: not configured' error")
	}
}
