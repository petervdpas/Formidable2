import { ref, onBeforeUnmount } from "vue";
import {
  Service as PluginSvc,
  type ListResult,
  type Command,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import type { MenuAction, MenuGroup } from "../types/menu";
import { useToast } from "./useToast";

// useWorkspacePluginMenu returns a reactive `pluginGroup` that any
// workspace can splice into its setTopbarMenu() getter. The group
// reflects every discovered plugin whose manifest attaches to the
// given workspace id — one menu item per command, label taken from
// the manifest's command.label.
//
// Empty list → null group → spread into [] folds away cleanly so the
// workspace doesn't even render a "Plugins" header when nothing is
// attached. Refresh fires automatically on `formidable:context-reloaded`
// (the same event templates/profiles/storage already listen to) so a
// pull/clone/reclone surfaces newly-installed plugins without a
// workspace remount.

export function useWorkspacePluginMenu(workspaceID: string) {
  const plugins = ref<ListResult[]>([]);
  const toast = useToast();

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

  async function runCommand(p: ListResult, cmd: Command): Promise<void> {
    try {
      const res = await PluginSvc.Run(p.id, cmd.id, { workspace: workspaceID });
      for (const ev of res.toasts ?? []) {
        const fn = toast[ev.level as "info" | "success" | "warn" | "error"];
        if (fn) fn(ev.message);
      }
      if (cmd.log_as_toast) {
        for (const line of res.logLines ?? []) {
          const m = /^\[(\w+)\]\s*(.*)$/.exec(line);
          const level = (m?.[1] ?? "info").toLowerCase();
          const msg = m?.[2] ?? line;
          const variant: "info" | "success" | "warn" | "error" =
            level === "warn"
              ? "warn"
              : level === "error"
                ? "error"
                : "info";
          toast[variant](msg);
        }
      }
      if (res.kind && res.kind !== "ok" && res.message) {
        toast.error(res.message);
      }
    } catch (err) {
      toast.error(String(err));
    }
  }

  function buildMenu(): MenuGroup | null {
    if (plugins.value.length === 0) return null;
    const items: MenuAction[] = [];
    for (const p of plugins.value) {
      const cmds = p.manifest.commands ?? [];
      const pluginLabel = p.manifest.name || p.id;
      const multi = cmds.length > 1;
      for (const cmd of cmds) {
        const cmdLabel = cmd.label || cmd.id;
        items.push({
          id: `${p.id}.${cmd.id}`,
          labelKey: "menu.plugins.workspace.item",
          label: multi ? `${pluginLabel} — ${cmdLabel}` : pluginLabel,
          onClick: () => runCommand(p, cmd),
        });
      }
    }
    if (items.length === 0) return null;
    return {
      type: "group",
      id: "plugins-workspace",
      labelKey: "menu.plugins.workspace.title",
      items,
    };
  }

  return { plugins, refresh, buildMenu };
}
