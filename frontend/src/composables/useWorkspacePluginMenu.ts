import { ref, watch, onBeforeUnmount } from "vue";
import {
  Service as PluginSvc,
  type ListResult,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { MenuAction, MenuGroup } from "../types/menu";
import { openGlobalPluginRun, useGlobalPluginRun } from "./useGlobalPluginRun";
import { pluginName } from "../utils/pluginI18n";

// useWorkspacePluginMenu returns a reactive `buildMenu()` that any
// workspace can call inside its setTopbarMenu() getter. The result is
// a "Plugins" group containing one item per plugin the backend says
// belongs in this workspace given the active template selection.
// Clicking an item opens the global PluginRunDialog with the plugin
// and a workspace ctx - the dialog renders modal-mode cards or
// form-mode fields based on the manifest's run_mode.
//
// Two channels combine on the backend (plugin.ListForTemplate):
//   - plain workspace plugins (no `templates`) always show;
//   - template-scoped plugins (manifest `templates`) show only while
//     one of their templates is the active selection.
// The match lives in Go; this composable just re-queries when the
// selected template changes and renders whatever comes back.
//
// `selectedTemplate`, when supplied, is a reactive getter for the
// workspace's current template filename (e.g. Storage passes its
// selectedTemplate). It drives both the backend filter and the
// click-time ctx. Template-less workspaces (profiles, collaboration,
// information) omit it: the backend then returns only the workspace
// channel. Empty list -> null group -> spreads into [] cleanly.
// Refresh also fires on `formidable:context-reloaded`.

type TemplateGetter = () => string;

export function useWorkspacePluginMenu(
  workspaceID: string,
  selectedTemplate?: TemplateGetter,
) {
  const plugins = ref<ListResult[]>([]);
  const { running } = useGlobalPluginRun();

  function currentTemplate(): string {
    return selectedTemplate ? selectedTemplate() : "";
  }

  async function refresh(): Promise<void> {
    try {
      plugins.value = await PluginSvc.ListForTemplate(
        workspaceID,
        currentTemplate(),
      );
    } catch {
      plugins.value = [];
    }
  }
  void refresh();

  // Re-query whenever the active template changes so a template-scoped
  // plugin appears/disappears as the user moves between records.
  if (selectedTemplate) {
    watch(selectedTemplate, () => {
      void refresh();
    });
  }

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
    const tpl = currentTemplate();
    const items: MenuAction[] = plugins.value.map((p) => ({
      id: p.id,
      labelKey: "menu.plugins.workspace.item",
      label: pluginName(p),
      disabled: isRunning,
      onClick: () => {
        const extra: Record<string, unknown> = { workspace: workspaceID };
        if (tpl) extra.template = tpl;
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
