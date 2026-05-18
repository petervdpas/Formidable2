import { ref, onMounted, onUnmounted } from "vue";
import { Events } from "@wailsio/runtime";
import { Service as PluginSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { ListResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// Global plugin-run dialog state. The workspace topbar menu opens
// this when clicked on any plugin — the same dialog the Plugins
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
// `progress` streams live `formidable.progress.tick` events from the
// running plugin via the Wails event channel. Cleared when a run
// finishes (success, error, or cancelled) so the next open doesn't
// flash stale ticks.

interface OpenRequest {
  plugin: ListResult;
  extraCtx?: Record<string, unknown>;
}

interface ProgressTick {
  done: number;
  total: number;
  stage: string;
  message: string;
}

const openRequest = ref<OpenRequest | null>(null);
const running = ref(false);
const stopping = ref(false);
const progress = ref<ProgressTick | null>(null);

let eventRefcount = 0;
let unsubscribe: (() => void) | null = null;

function ensureSubscription() {
  if (eventRefcount === 0 && !unsubscribe) {
    unsubscribe = Events.On("plugin:progress", (evt: unknown) => {
      // Wails wraps payloads as { data: <go-struct>, ... }; tolerate
      // both shapes so a binding change doesn't break this.
      const raw =
        evt && typeof evt === "object" && "data" in evt
          ? (evt as { data: unknown }).data
          : evt;
      const e = raw as Partial<ProgressTick> | undefined;
      if (!e) return;
      progress.value = {
        done: Number(e.done ?? 0),
        total: Number(e.total ?? 0),
        stage: String(e.stage ?? ""),
        message: String(e.message ?? ""),
      };
    });
  }
  eventRefcount += 1;
}

function releaseSubscription() {
  eventRefcount = Math.max(0, eventRefcount - 1);
  if (eventRefcount === 0 && unsubscribe) {
    unsubscribe();
    unsubscribe = null;
  }
}

export function openGlobalPluginRun(
  plugin: ListResult,
  extraCtx?: Record<string, unknown>,
): boolean {
  if (running.value) return false;
  progress.value = null;
  openRequest.value = { plugin, extraCtx };
  return true;
}

export function closeGlobalPluginRun(): void {
  openRequest.value = null;
  progress.value = null;
  stopping.value = false;
}

export function setGlobalPluginRunning(v: boolean): void {
  running.value = v;
  if (!v) {
    progress.value = null;
    stopping.value = false;
  }
}

export async function cancelGlobalPluginRun(): Promise<void> {
  // Flip stopping immediately so the Stop button can disable + show
  // a "Stopping…" label while the IPC roundtrip lands. Cleared in
  // setGlobalPluginRunning(false) when Run resolves (success, error,
  // or cancelled — every path goes through that finally branch).
  stopping.value = true;
  try {
    await PluginSvc.Cancel();
  } catch {
    // Best-effort — the Run path will surface kind="cancelled" when
    // the cancel actually lands. Swallowing here keeps the UI from
    // showing two errors for one user action.
  }
}

export function useGlobalPluginRun() {
  onMounted(ensureSubscription);
  onUnmounted(releaseSubscription);
  return { openRequest, running, stopping, progress };
}
