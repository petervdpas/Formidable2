import { onBeforeUnmount, onMounted } from "vue";
import { Events } from "@wailsio/runtime";
import { useI18n } from "vue-i18n";
import { useToast } from "./useToast";
import { useActiveWorkspace } from "./useActiveWorkspace";
import { useInformationSection } from "./useInformationSection";
import { confirmLeave } from "./useNavGuard";
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
// is sticky and routes to the API-clients panel. Installs also happen at boot,
// before any window exists, so the backend queues them and this drains the
// queue on mount and after every plugin refresh. A running script can create
// a client too (formidable.client.create); the backend signals that over the
// "plugin:clients" Wails event so the toast lands while the run is still up.
export function usePluginClientNotices(): void {
  const { t } = useI18n();
  const toast = useToast();
  const { setActive: setWorkspace } = useActiveWorkspace();
  const { setActive: setSection } = useInformationSection();

  async function openClients(): Promise<void> {
    if (!(await confirmLeave())) return; // honor an unsaved-changes guard
    setWorkspace("information");
    setSection("api-clients");
  }

  function notify(n: ClientInstall): void {
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
        action: {
          label: t("workspace.plugins.client.action"),
          run: () => void openClients(),
        },
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

  async function drain(reported: ClientInstall[] = []): Promise<void> {
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
