import { ref, onBeforeUnmount } from "vue";
import {
  Service as PluginSvc,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { MenuAction, MenuGroup } from "../types/menu";
import { openGlobalPluginRun, useGlobalPluginRun } from "./useGlobalPluginRun";

// useWorkspacePluginMenu returns a reactive `buildMenu()` that any
// workspace can call inside its setTopbarMenu() getter. The result is
// a "Plugins" group containing one item per plugin whose manifest
// attaches to the given workspace id. Clicking an item opens the
// global PluginRunDialog with the plugin and a workspace ctx — the
// dialog then renders modal-mode cards or form-mode fields based on
// the manifest's run_mode, exactly mirroring the in-workspace Run UI.
//
// Empty list → null group → spread into [] folds away cleanly so the
// workspace doesn't render a "Plugins" header when nothing is
// attached. Refresh fires on `formidable:context-reloaded` so a
// pull/clone surfaces newly-installed plugins without a remount.
//
// `selectionFeeder`, when supplied, is called at click time to mint
// the workspace's current selection state — e.g. the Storage
// workspace passes { template: <filename> } so a plugin scoped to
// "this template" (like wikiwonder) gets the selection in its ctx
// without having to enumerate the catalog.

type SelectionFeeder = () => Record<string, unknown>;

export function useWorkspacePluginMenu(
  workspaceID: string,
  selectionFeeder?: SelectionFeeder,
) {
  const plugins = ref<ListResult[]>([]);
  const { running } = useGlobalPluginRun();

  async function refresh(): Promise<void> {
    try {
      plugins.value = await PluginSvc.ListForWorkspace(workspaceID);
    } catch {
      plugins.value = [];
    }
  }
  void refresh();

  if (typeof window !== "undefined") {
    const handler = () => {
      void refresh();
    };
    window.addEventListener("formidable:context-reloaded", handler);
    onBeforeUnmount(() => {
      window.removeEventListener("formidable:context-reloaded", handler);
    });
  }

  function buildMenu(): MenuGroup | null {
    if (plugins.value.length === 0) return null;
    const isRunning = running.value;
    const items: MenuAction[] = plugins.value.map((p) => ({
      id: p.id,
      labelKey: "menu.plugins.workspace.item",
      label: p.manifest.name || p.id,
      disabled: isRunning,
      onClick: () => {
        const extra: Record<string, unknown> = { workspace: workspaceID };
        if (selectionFeeder) {
          const sel = selectionFeeder();
          for (const k of Object.keys(sel)) extra[k] = sel[k];
        }
        openGlobalPluginRun(p, extra);
      },
    }));
    return {
      type: "group",
      id: "plugins-workspace",
      labelKey: "menu.plugins.workspace.title",
      items,
    };
  }

  return { plugins, refresh, buildMenu };
}
