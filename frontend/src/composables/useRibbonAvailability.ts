import { ref, watch } from "vue";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { useTemplates } from "./useTemplates";
import { useProfiles } from "./useProfiles";
import { useConfig } from "./useConfig";
import type { WorkspaceId } from "../workspaces";

// Single source of truth for ribbon availability — used by Ribbon.vue
// to ghost disabled items and by App.vue to redirect away from a
// workspace that becomes disabled at boot or during the session.
//
// Backend owns the rule (template.HasTemplates / config.HasUserProfiles);
// the config-flag rule (enable_plugins) is read directly from the
// reactive Config ref because it's already a user-config field. We
// re-fetch the backend booleans whenever the underlying composable
// lists change so create/delete in another workspace propagates here
// without explicit cross-coupling.

const hasTemplates = ref(true);
const hasProfiles = ref(true);
let booted = false;

// fallbackFor: where to send the user when their active workspace
// becomes unavailable. Each entry encodes "you can't do X without
// first doing Y" — Storage needs a template, Settings needs a
// profile, Plugins needs the feature flag (toggled in Settings),
// Collaboration needs a remote backend (configured in Settings →
// Locations). Add new entries here as more conditional ribbon items
// land.
const FALLBACK: Partial<Record<WorkspaceId, WorkspaceId>> = {
  storage: "templates",
  settings: "profiles",
  plugins: "settings",
  collaboration: "settings",
};

export function useRibbonAvailability() {
  const { filenames: templateFilenames } = useTemplates();
  const { profiles } = useProfiles();
  const { config } = useConfig();

  async function refresh(): Promise<void> {
    [hasTemplates.value, hasProfiles.value] = await Promise.all([
      TemplateSvc.HasTemplates(),
      ConfigSvc.HasUserProfiles(),
    ]);
  }

  if (!booted) {
    booted = true;
    void refresh();
    watch([templateFilenames, profiles], () => void refresh(), { deep: true });
  }

  function isDisabled(id: WorkspaceId): boolean {
    if (id === "storage") return !hasTemplates.value;
    if (id === "settings") return !hasProfiles.value;
    if (id === "plugins") return !config.value?.enable_plugins;
    // Collaboration is meaningful only when a remote backend is
    // configured. "none" (the default) means the user is purely
    // local — ghost the ribbon and redirect them to Settings →
    // Locations to pick Git or GiGot first.
    if (id === "collaboration") {
      return (config.value?.remote_backend ?? "none") === "none";
    }
    return false;
  }

  function fallbackFor(id: WorkspaceId): WorkspaceId | null {
    return FALLBACK[id] ?? null;
  }

  return { hasTemplates, hasProfiles, isDisabled, fallbackFor, refresh };
}
