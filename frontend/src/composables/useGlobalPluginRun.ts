import { ref } from "vue";
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

interface OpenRequest {
  plugin: ListResult;
  extraCtx?: Record<string, unknown>;
}

const openRequest = ref<OpenRequest | null>(null);
const running = ref(false);

export function openGlobalPluginRun(
  plugin: ListResult,
  extraCtx?: Record<string, unknown>,
): boolean {
  if (running.value) return false;
  openRequest.value = { plugin, extraCtx };
  return true;
}

export function closeGlobalPluginRun(): void {
  openRequest.value = null;
}

export function setGlobalPluginRunning(v: boolean): void {
  running.value = v;
}

export function useGlobalPluginRun() {
  return { openRequest, running };
}
