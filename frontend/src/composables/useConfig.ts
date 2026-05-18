import { ref, type Ref } from "vue";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { Config } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";

const config: Ref<Config | null> = ref(null);
const profileFilename = ref<string>("");
let loadPromise: Promise<void> | null = null;

function load(): Promise<void> {
  if (loadPromise) return loadPromise;
  loadPromise = (async () => {
    const [cfg, fn] = await Promise.all([
      ConfigSvc.LoadUserConfig(),
      ConfigSvc.CurrentProfileFilename(),
    ]);
    config.value = cfg;
    profileFilename.value = fn;
  })();
  return loadPromise;
}

async function update(partial: Record<string, unknown>): Promise<void> {
  const updated = await ConfigSvc.UpdateUserConfig(partial);
  if (updated) config.value = updated;
}

async function reload(): Promise<void> {
  loadPromise = null;
  await load();
}

// See useTemplates: pull/clone/reclone fires this event after writing
// to the context folder. The active profile JSON itself can be
// overwritten by a sync, so re-read it.
if (typeof window !== "undefined") {
  window.addEventListener("formidable:context-reloaded", () => {
    if (loadPromise) void reload();
  });
}

// switchProfile flips .boot.json to the given filename via the backend
// (which is serialized against UpdateUserConfig under updateMu) and
// then refreshes our reactive cache. Watchers on config.theme,
// config.language, etc. fire automatically — no full window reload.
async function switchProfile(filename: string): Promise<void> {
  const updated = await ConfigSvc.SwitchUserProfile(filename);
  if (updated) {
    config.value = updated;
    profileFilename.value = await ConfigSvc.CurrentProfileFilename();
  }
}

export function useConfig() {
  if (!loadPromise) load();
  return { config, profileFilename, update, reload, switchProfile };
}

export { Config };
