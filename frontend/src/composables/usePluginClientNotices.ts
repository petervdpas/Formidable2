import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { useToast } from "./useToast";
import { useI18nLoader } from "./useI18nLoader";
import { useTemplates } from "./useTemplates";
import {
  Service as PluginSvc,
  type ClientInstall,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";

// Fired by usePlugins after a refresh or an import. `detail` carries the
// outcomes an import already holds; the backend queue holds the rest.
export const PLUGIN_CLIENTS_EVENT = "formidable:plugin-clients";

// usePluginClientNotices is the app-shell watcher that tells the user what
// happened to the api-clients a plugin ships. A plugin never carries a
// credential, so a freshly created client cannot call anything yet: that toast
// is sticky, text only. A client is only ever created
// on request (zip import, the editor's create button, a script); the backend
// queues the outcome and this drains the queue. A running script can create
// a client too (formidable.client.create); the backend signals that over the
// "plugin:clients" Wails event so the toast lands while the run is still up.
export function usePluginClientNotices(): void {
  const toast = useToast();
  const { refresh: refreshTemplates } = useTemplates();

  // A template a plugin created (on import or from its script). The list of
  // templates is re-read so it shows up without a reload.
  function notifyTemplate(n: ClientInstall): void {
    const dedupeKey = `plugin-template|${n.plugin}|${n.id}|${n.status}`;
    if (n.status === "installed") {
      void refreshTemplates();
      toast.info("workspace.plugins.heldtemplates.notice_installed", [n.plugin, n.id], { dedupeKey });
    } else if (n.status === "exists" && n.differs) {
      toast.warn("workspace.plugins.heldtemplates.notice_differs", [n.plugin, n.id], { sticky: true, dedupeKey });
    } else if (n.status === "exists") {
      toast.info("workspace.plugins.heldtemplates.notice_exists", [n.plugin, n.id], { dedupeKey });
    } else if (n.status === "failed") {
      toast.error("workspace.plugins.heldtemplates.notice_failed", [n.plugin, n.id, n.error ?? ""], {
        sticky: true,
        dedupeKey,
      });
    }
  }

  function notify(n: ClientInstall): void {
    if (n.kind === "template") {
      notifyTemplate(n);
      return;
    }
    const dedupeKey = `plugin-client|${n.plugin}|${n.id}|${n.status}`;
    if (n.status === "installed") {
      // A derived client was drafted from the document alone, so its
      // bindings want a look too, not only its credentials.
      const key = n.derived
        ? "workspace.plugins.client.installed_derived"
        : "workspace.plugins.client.installed";
      toast.warn(key, [n.plugin, n.id], {
        sticky: true,
        force: true,
        dedupeKey,
      });
    } else if (n.status === "exists") {
      toast.info("workspace.plugins.client.exists", [n.plugin, n.id], { dedupeKey });
    } else if (n.status === "failed") {
      toast.error("workspace.plugins.client.failed", [n.plugin, n.id, n.error ?? ""], {
        sticky: true,
        dedupeKey,
      });
    }
  }

  const { ensureI18nReady } = useI18nLoader();

  async function drain(reported: ClientInstall[] = []): Promise<void> {
    // A toast is translated when it is raised. The boot drain runs at mount,
    // which can be before the first locale bundle lands, and would then show
    // raw keys.
    try {
      await ensureI18nReady();
    } catch {
      // Raw keys beat a notice that never shows.
    }
    let pending: ClientInstall[] = [];
    try {
      pending = (await PluginSvc.TakeClientNotices()) ?? [];
    } catch {
      // Nothing to report is the safe reading of a failed drain.
    }
    for (const n of [...reported, ...pending]) notify(n);
  }

  function onEvent(e: Event): void {
    void drain((e as CustomEvent<ClientInstall[]>).detail ?? []);
  }

  let unsubscribe: (() => void) | undefined;

  onMounted(() => {
    window.addEventListener(PLUGIN_CLIENTS_EVENT, onEvent);
    unsubscribe = Events.On("plugin:clients", () => void drain());
    void drain();
  });
  onBeforeUnmount(() => {
    window.removeEventListener(PLUGIN_CLIENTS_EVENT, onEvent);
    unsubscribe?.();
  });
}
