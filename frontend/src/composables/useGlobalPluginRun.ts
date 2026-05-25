import { ref, onMounted, onUnmounted } from "vue";
import { Events } from "@wailsio/runtime";
import { Service as PluginSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { ListResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// Global plugin-run dialog state. The workspace topbar menu opens
// this when clicked on any plugin - the same dialog the Plugins
// workspace's Run button uses, but mounted at the App level so it's
// reachable from any workspace.
//
// `extraCtx` rides along into every Lua call as part of `ctx` so the
// plugin sees both the user-filled form values and any workspace
// context the menu supplied (e.g. `{ workspace: "storage" }`).
//
// `running` is the one-at-a-time guard. Any pipeline (this dialog
// AND the Plugins workspace's inline Run modal) flips it true on the
// Lua call boundary; openGlobalPluginRun refuses while it's true and
// workspace topbar menu items read it to compute their disabled state.
//
// `bar` and `status` stream live updates from the running plugin via
// the Wails event channel (plugin:run:bar / plugin:run:status). They
// are independent - the Lua side calls formidable.run.bar or
// formidable.run.status as it likes - and feed any progressbar or
// statusmessage widget the plugin author dropped into their form.
// Both are cleared at the start of every run so the next open
// doesn't flash stale state.

interface OpenRequest {
  plugin: ListResult;
  extraCtx?: Record<string, unknown>;
}

interface RunBar {
  done: number;
  total: number;
}

// RunChartSpec is the chart envelope a plugin pushes via
// formidable.run.chart(spec): `type` is the chart shape (StatChart
// override) and `result` is the stat Result the chart widget renders.
interface RunChartSpec {
  type?: string;
  title?: string;
  result?: unknown;
  // The template's facets, when the plugin passes them, so the chart
  // colors categories with the facets' authored option colors.
  facets?: unknown;
}

const openRequest = ref<OpenRequest | null>(null);
const running = ref(false);
const stopping = ref(false);
const bar = ref<RunBar | null>(null);
const status = ref<string>("");
const chart = ref<RunChartSpec | null>(null);
// fieldOptions overlays a form field's option list at runtime, keyed
// by field key. Fed by formidable.run.options so a plugin can steer
// e.g. the shape dropdown's choices from the picked object's rank.
// Cleared on open/close, NOT on run start (an on-change run sets it).
const fieldOptions = ref<Record<string, unknown[]>>({});

let eventRefcount = 0;
let unsubscribeBar: (() => void) | null = null;
let unsubscribeStatus: (() => void) | null = null;
let unsubscribeChart: (() => void) | null = null;
let unsubscribeOptions: (() => void) | null = null;

function unwrap(evt: unknown): unknown {
  // Wails wraps payloads as { data: <go-struct>, ... }; tolerate
  // both shapes so a binding-runtime change doesn't break this.
  if (evt && typeof evt === "object" && "data" in evt) {
    return (evt as { data: unknown }).data;
  }
  return evt;
}

function ensureSubscription() {
  if (eventRefcount === 0) {
    if (!unsubscribeBar) {
      unsubscribeBar = Events.On("plugin:run:bar", (evt: unknown) => {
        const e = unwrap(evt) as Partial<RunBar> | undefined;
        if (!e) return;
        bar.value = {
          done: Number(e.done ?? 0),
          total: Number(e.total ?? 0),
        };
      });
    }
    if (!unsubscribeStatus) {
      unsubscribeStatus = Events.On("plugin:run:status", (evt: unknown) => {
        const e = unwrap(evt) as { text?: string } | undefined;
        if (!e) return;
        status.value = String(e.text ?? "");
      });
    }
    if (!unsubscribeChart) {
      unsubscribeChart = Events.On("plugin:run:chart", (evt: unknown) => {
        const e = unwrap(evt) as { spec?: RunChartSpec } | undefined;
        chart.value = e?.spec ?? null;
      });
    }
    if (!unsubscribeOptions) {
      unsubscribeOptions = Events.On("plugin:run:options", (evt: unknown) => {
        const e = unwrap(evt) as { field?: string; options?: unknown[] } | undefined;
        if (!e?.field) return;
        fieldOptions.value = { ...fieldOptions.value, [e.field]: e.options ?? [] };
      });
    }
  }
  eventRefcount += 1;
}

function releaseSubscription() {
  eventRefcount = Math.max(0, eventRefcount - 1);
  if (eventRefcount === 0) {
    if (unsubscribeBar) {
      unsubscribeBar();
      unsubscribeBar = null;
    }
    if (unsubscribeStatus) {
      unsubscribeStatus();
      unsubscribeStatus = null;
    }
    if (unsubscribeChart) {
      unsubscribeChart();
      unsubscribeChart = null;
    }
    if (unsubscribeOptions) {
      unsubscribeOptions();
      unsubscribeOptions = null;
    }
  }
}

export function openGlobalPluginRun(
  plugin: ListResult,
  extraCtx?: Record<string, unknown>,
): boolean {
  if (running.value) return false;
  bar.value = null;
  status.value = "";
  chart.value = null;
  fieldOptions.value = {};
  openRequest.value = { plugin, extraCtx };
  return true;
}

export function closeGlobalPluginRun(): void {
  openRequest.value = null;
  bar.value = null;
  status.value = "";
  chart.value = null;
  fieldOptions.value = {};
  stopping.value = false;
}

export function setGlobalPluginRunning(v: boolean): void {
  if (v) {
    // Starting a run - wipe stale tick state so the previous run's
    // last bar/status values don't flash before the first new tick.
    bar.value = null;
    status.value = "";
    chart.value = null;
  }
  running.value = v;
  if (!v) {
    stopping.value = false;
  }
}

export async function cancelGlobalPluginRun(): Promise<void> {
  // Flip stopping immediately so the Stop button can disable + show
  // a "Stopping…" label while the IPC roundtrip lands. Cleared in
  // setGlobalPluginRunning(false) when Run resolves (success, error,
  // or cancelled - every path goes through that finally branch).
  stopping.value = true;
  try {
    await PluginSvc.Cancel();
  } catch {
    // Best-effort - the Run path will surface kind="cancelled" when
    // the cancel actually lands. Swallowing here keeps the UI from
    // showing two errors for one user action.
  }
}

export function useGlobalPluginRun() {
  onMounted(ensureSubscription);
  onUnmounted(releaseSubscription);
  return { openRequest, running, stopping, bar, status, chart, fieldOptions };
}
