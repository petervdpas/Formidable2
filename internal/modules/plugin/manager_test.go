package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// newTestManager builds a Manager rooted at a fresh temp dir and
// pre-wired with a real *KV plus the in-test mocks. Each test
// adds plugin folders under deps.PluginsDir before calling Refresh.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
	})
	return m, pluginsDir
}

func TestManager_Refresh_EmptyDir(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Refresh(); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("got %d plugins, want 0", len(got))
	}
}

func TestManager_Refresh_DiscoversValidPlugin(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run(ctx) return 42 end")
	if err := m.Refresh(); err != nil {
		t.Fatalf("err: %v", err)
	}
	got := m.List()
	if len(got) != 1 || got[0].Manifest.ID != "demo" {
		t.Fatalf("got %+v", got)
	}
}

func TestManager_Refresh_SkipsLooseFiles(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	// Stray file at plugins root — should be ignored.
	if err := os.WriteFile(filepath.Join(pluginsDir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	if len(m.List()) != 1 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsFoldersWithoutManifest(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	// Folder without plugin.json — silently skipped.
	if err := os.MkdirAll(filepath.Join(pluginsDir, "noplugin"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = m.Refresh()
	if len(m.List()) != 0 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsHiddenAndKVDir(t *testing.T) {
	// `.kv` is the K/V root; anything starting with "." should be
	// skipped so plugin authors can store helper files alongside.
	m, pluginsDir := newTestManager(t)
	if err := os.MkdirAll(filepath.Join(pluginsDir, ".kv"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginsDir, ".cache"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = m.Refresh()
	if len(m.List()) != 0 {
		t.Fatalf("got %d", len(m.List()))
	}
}

func TestManager_Refresh_SkipsBadManifestKeepsValid(t *testing.T) {
	// Corrupt manifest in one folder must not crash the scan or
	// poison the others. The good plugin still loads.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "broken", `{not json`, "function run() end")
	writePlugin(t, pluginsDir, "good", `{
		"manifest_version": 1, "id": "good", "name": "Good",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	got := m.List()
	if len(got) != 1 || got[0].Manifest.ID != "good" {
		t.Fatalf("got %+v", got)
	}
}

func TestManager_ListForWorkspace_FiltersByManifest(t *testing.T) {
	// Three plugins:
	//   - "a" attaches to storage + templates
	//   - "b" attaches to storage only
	//   - "c" has no workspaces declared
	// ListForWorkspace("storage") returns {a, b}; ListForWorkspace("templates") returns {a}.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "a", `{
		"manifest_version": 1, "id": "a", "name": "A",
		"version": "0.1.0",
		"workspaces": ["storage", "templates"],
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePlugin(t, pluginsDir, "b", `{
		"manifest_version": 1, "id": "b", "name": "B",
		"version": "0.1.0",
		"workspaces": ["storage"],
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	writePlugin(t, pluginsDir, "c", `{
		"manifest_version": 1, "id": "c", "name": "C",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	if err := m.Refresh(); err != nil {
		t.Fatalf("err: %v", err)
	}

	gotStorage := m.ListForWorkspace(WorkspaceStorage)
	if len(gotStorage) != 2 || gotStorage[0].Manifest.ID != "a" || gotStorage[1].Manifest.ID != "b" {
		t.Fatalf("storage: %+v", ids(gotStorage))
	}
	gotTemplates := m.ListForWorkspace(WorkspaceTemplates)
	if len(gotTemplates) != 1 || gotTemplates[0].Manifest.ID != "a" {
		t.Fatalf("templates: %+v", ids(gotTemplates))
	}
	gotProfiles := m.ListForWorkspace(WorkspaceProfiles)
	if len(gotProfiles) != 0 {
		t.Fatalf("profiles should be empty: %+v", ids(gotProfiles))
	}
}

func TestManager_ListForWorkspace_RejectsUnknownAndEmpty(t *testing.T) {
	// Defensive: passing an unknown id or "" returns nil rather than
	// silently matching every unattached plugin.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "a", `{
		"manifest_version": 1, "id": "a", "name": "A",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() end")
	_ = m.Refresh()
	if got := m.ListForWorkspace(""); got != nil {
		t.Fatalf("empty ws should be nil, got %v", ids(got))
	}
	if got := m.ListForWorkspace("bogus"); got != nil {
		t.Fatalf("unknown ws should be nil, got %v", ids(got))
	}
}

// ids extracts just the ids from a Plugin slice for compact test diagnostics.
func ids(ps []Plugin) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Manifest.ID
	}
	return out
}

func TestManager_Run_UnknownPlugin(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.Run("ghost", "run", nil)
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestManager_Run_UnknownCommand(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	_, err := m.Run("demo", "ghost", nil)
	if !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestManager_Run_HappyReturnsValue(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run(ctx) return { value = 42 } end")
	_ = m.Refresh()
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["value"] != float64(42) {
		t.Fatalf("got %v", got)
	}
}

func TestManager_Run_PassesCtxArgument(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "echo", "label": "Echo"}]
	}`, "function echo(ctx) return ctx.greeting end")
	_ = m.Refresh()
	res, _ := m.Run("demo", "echo", map[string]any{"greeting": "hi"})
	if res.Value != "hi" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_ExplicitFnOverridesID(t *testing.T) {
	// Command with an explicit "fn" hits the named function instead
	// of one matching the command id.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "user_facing", "label": "Run", "fn": "actual_fn"}]
	}`, "function actual_fn() return 'right one' end")
	_ = m.Refresh()
	res, err := m.Run("demo", "user_facing", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "right one" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_RequiresInternalServer_FailsWhenDown(t *testing.T) {
	// Manifest declares it needs the internal server, but the
	// HTTPClient reports it as unavailable. Run must fail before
	// loading the script, with ErrServerNotRunning so the
	// frontend can dispatch a clean "start the server" toast.
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	api := &fakeAPI{running: false}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		API:        api,
	})
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"requires_internal_server": true,
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 1 end")
	_ = m.Refresh()
	_, err := m.Run("demo", "run", nil)
	if !errors.Is(err, ErrServerNotRunning) {
		t.Fatalf("got %v, want ErrServerNotRunning", err)
	}
}

func TestManager_Run_RequiresInternalServer_PassesWhenUp(t *testing.T) {
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	api := &fakeAPI{running: true}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		API:        api,
	})
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"requires_internal_server": true,
		"commands": [{"id": "run", "label": "Run"}]
	}`, "function run() return 'ok' end")
	_ = m.Refresh()
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "ok" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_KVScopedToPluginID(t *testing.T) {
	// A plugin's KV is keyed by the plugin id; two plugins setting
	// the same key see independent values.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "a", `{
		"manifest_version": 1, "id": "a", "name": "A",
		"version": "0.1.0",
		"commands": [{"id": "set", "label": "set"}, {"id": "get", "label": "get"}]
	}`, `
		function set(ctx) formidable.kv.set("k", "from-a") end
		function get(ctx) return formidable.kv.get("k") end`)
	writePlugin(t, pluginsDir, "b", `{
		"manifest_version": 1, "id": "b", "name": "B",
		"version": "0.1.0",
		"commands": [{"id": "set", "label": "set"}, {"id": "get", "label": "get"}]
	}`, `
		function set(ctx) formidable.kv.set("k", "from-b") end
		function get(ctx) return formidable.kv.get("k") end`)
	_ = m.Refresh()
	_, _ = m.Run("a", "set", nil)
	_, _ = m.Run("b", "set", nil)
	gotA, _ := m.Run("a", "get", nil)
	gotB, _ := m.Run("b", "get", nil)
	if gotA.Value != "from-a" || gotB.Value != "from-b" {
		t.Fatalf("isolation broken: a=%v b=%v", gotA.Value, gotB.Value)
	}
}

func TestManager_Run_BusyRejectsConcurrent(t *testing.T) {
	// While one Run is in flight (Lua is parked on a coroutine.yield
	// equivalent — here we use a channel that the test holds open),
	// a second Run on any plugin must fail fast with ErrPluginBusy.
	// We use a Lua loop polling a `formidable.kv.get` for a sentinel
	// the test writes — keeps the script alive until we release it.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "slow", `{
		"manifest_version": 1, "id": "slow", "name": "Slow",
		"version": "0.1.0",
		"commands": [{"id": "loop", "label": "loop"}]
	}`, `
		function loop()
			while formidable.kv.get("go") ~= "yes" do
				-- spin
			end
			return "done"
		end`)
	writePlugin(t, pluginsDir, "fast", `{
		"manifest_version": 1, "id": "fast", "name": "Fast",
		"version": "0.1.0",
		"commands": [{"id": "ping", "label": "ping"}]
	}`, "function ping() return 'ok' end")
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Kick off the slow Run on a goroutine; wait for it to actually
	// enter the script before testing the second call.
	slowStarted := make(chan struct{})
	slowDone := make(chan struct{})
	go func() {
		// The CAS happens at the very top of Run, so once the script
		// is spinning we know the flag is set. Signal once we know
		// we're past the CAS by writing a sentinel inside the loop
		// — except we can't from outside. Easier: yield first via
		// a poll-loop write from the test (next line). Simpler still
		// — small sleep to allow the goroutine to enter Run.
		close(slowStarted)
		_, _ = m.Run("slow", "loop", nil)
		close(slowDone)
	}()
	<-slowStarted
	// Wait until the goroutine has actually entered Run + set the
	// runActive flag. Without this barrier, the second Run below
	// races against the first and may not hit the CAS contention
	// case we're testing.
	deadline := time.Now().Add(2 * time.Second)
	for !m.runActive.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if !m.runActive.Load() {
		t.Fatalf("slow Run never set runActive")
	}

	// While slow is in flight, fast Run must fail with ErrPluginBusy.
	_, err := m.Run("fast", "ping", nil)
	if !errors.Is(err, ErrPluginBusy) {
		t.Fatalf("concurrent Run should return ErrPluginBusy, got %v", err)
	}

	// Release the slow script and wait for it to complete.
	_ = m.deps.KV.Set("slow", "go", "yes")
	<-slowDone

	// After completion, a new Run succeeds.
	res, err := m.Run("fast", "ping", nil)
	if err != nil {
		t.Fatalf("post-busy Run: %v", err)
	}
	if res.Value != "ok" {
		t.Fatalf("got %v", res.Value)
	}
}

func TestManager_Run_BusyClearedOnError(t *testing.T) {
	// A Run that fails (unknown command, script error, etc.) must
	// still release the busy flag so the next Run isn't blocked.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "demo", `{
		"manifest_version": 1, "id": "demo", "name": "Demo",
		"version": "0.1.0",
		"commands": [{"id": "boom", "label": "boom"}]
	}`, `function boom() error("nope") end`)
	_ = m.Refresh()

	_, err := m.Run("demo", "boom", nil)
	if err == nil {
		t.Fatal("expected runtime error from boom")
	}
	if m.runActive.Load() {
		t.Fatal("runActive should be cleared after a failed Run")
	}
	// Second Run still works.
	_, err2 := m.Run("demo", "ghost", nil)
	if !errors.Is(err2, ErrCommandNotFound) {
		t.Fatalf("second Run: want ErrCommandNotFound, got %v", err2)
	}
	if m.runActive.Load() {
		t.Fatal("runActive should be cleared after ErrCommandNotFound")
	}
}

func TestManager_Cancel_AbortsRunningPlugin(t *testing.T) {
	// While a Run is spinning on a KV-poll, Cancel() must fire the
	// run's context and surface ErrPluginCancelled.
	m, pluginsDir := newTestManager(t)
	writePlugin(t, pluginsDir, "spin", `{
		"manifest_version": 1, "id": "spin", "name": "Spin",
		"version": "0.1.0",
		"commands": [{"id": "loop", "label": "loop"}]
	}`, `
		function loop()
			while true do
				-- pure spin; SetContext aborts between instructions
			end
		end`)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	type runResult struct {
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		_, err := m.Run("spin", "loop", nil)
		done <- runResult{err: err}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !m.runActive.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if !m.runActive.Load() {
		t.Fatalf("Run never entered VM (runActive still false)")
	}
	m.Cancel()
	select {
	case res := <-done:
		if !errors.Is(res.err, ErrPluginCancelled) {
			t.Fatalf("Run after Cancel: err = %v, want ErrPluginCancelled", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Cancel()")
	}
	if m.runActive.Load() {
		t.Fatal("runActive should be cleared after cancelled Run")
	}
}

func TestManager_Cancel_NoActiveRun_IsNoOp(t *testing.T) {
	m, _ := newTestManager(t)
	// No Run in flight; Cancel must not panic or block.
	m.Cancel()
}

func TestManager_FormValues_EmptyOnFreshPlugin(t *testing.T) {
	m, _ := newTestManager(t)
	got := m.LoadFormValues("never-saved", []string{"input", "what"})
	if got == nil {
		t.Fatal("LoadFormValues should return empty map, not nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestManager_FormValues_RoundTrip(t *testing.T) {
	m, _ := newTestManager(t)
	want := map[string]any{
		"input": "/tmp/x.bat",
		"what":  "hello",
		"flag":  true,
	}
	if err := m.SaveFormValues("p", want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := m.LoadFormValues("p", []string{"input", "what", "flag"})
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("key %q: got %v, want %v", k, got[k], v)
		}
	}
}

func TestManager_FormValues_PerPluginIsolation(t *testing.T) {
	m, _ := newTestManager(t)
	_ = m.SaveFormValues("a", map[string]any{"x": "from-a"})
	_ = m.SaveFormValues("b", map[string]any{"x": "from-b"})
	if m.LoadFormValues("a", []string{"x"})["x"] != "from-a" {
		t.Fatalf("a leaked")
	}
	if m.LoadFormValues("b", []string{"x"})["x"] != "from-b" {
		t.Fatalf("b leaked")
	}
}

func TestManager_FormValues_VisibleFromLuaKV(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	// Save form values via the Vue path, then have a Lua script
	// read them back via formidable.kv.get(fieldKey). They must
	// land in the same KV bag — that's the whole point of the
	// shared key namespace.
	_ = m.SaveFormValues("p", map[string]any{
		"input": "/tmp/x.bat",
		"what":  "hello",
	})
	writePlugin(t, pluginsDir, "p", `{
		"manifest_version": 1, "id": "p", "name": "P",
		"version": "0.1.0",
		"commands": [{"id": "read", "label": "read"}]
	}`, `
		function read(ctx)
			return {
				input = formidable.kv.get("input"),
				what  = formidable.kv.get("what"),
			}
		end`)
	_ = m.Refresh()
	res, err := m.Run("p", "read", nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T (%v)", res.Value, res.Value)
	}
	if got["input"] != "/tmp/x.bat" || got["what"] != "hello" {
		t.Fatalf("Lua kv.get didn't see SaveFormValues entries: %v", got)
	}
}

func TestManager_FormValues_PreservesUnrelatedKVEntries(t *testing.T) {
	m, _ := newTestManager(t)
	// Plugin author had set "counter" via Lua kv.set. SaveFormValues
	// should write only the form fields it was given and leave any
	// other plugin-authored slots untouched.
	_ = m.deps.KV.Set("p", "counter", 42)
	_ = m.SaveFormValues("p", map[string]any{"input": "/tmp/x"})
	got, ok, _ := m.deps.KV.Get("p", "counter")
	if !ok || got != float64(42) && got != 42 {
		t.Fatalf("counter clobbered: %v ok=%v", got, ok)
	}
}
