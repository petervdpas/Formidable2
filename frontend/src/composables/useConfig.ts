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

export function useConfig() {
  if (!loadPromise) load();
  return { config, profileFilename, update, reload };
}

export { Config };
