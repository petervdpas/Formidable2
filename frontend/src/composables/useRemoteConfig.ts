import { computed } from "vue";
import { useConfig } from "./useConfig";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";

// useRemoteConfig is the single place the collaboration UI reads the active
// profile's remote settings. The nested config shape (git/gigot blocks) and the
// one shared root resolution live here, so no component re-derives them and the
// schema can change in exactly one file.
export function useRemoteConfig() {
  const { config, update } = useConfig();

  const backend = computed(() => config.value?.remote_backend ?? "none");
  const contextFolder = computed(() => config.value?.context_folder ?? "");

  // The absolute working folder, resolved the one same way for every backend
  // (none/git/gigot) via the backend resolver; falls back to the raw value.
  async function resolveRoot(): Promise<string> {
    return (await ConfigSvc.GetRemoteRootPath()) || contextFolder.value;
  }

  const gigotBaseURL = computed(() => config.value?.gigot?.base_url ?? "");
  const gigotRepoName = computed(() => config.value?.gigot?.repo_name ?? "");
  const gitBranch = computed(() => config.value?.git?.branch ?? "");

  return {
    config,
    update,
    backend,
    contextFolder,
    resolveRoot,
    gigotBaseURL,
    gigotRepoName,
    gitBranch,
  };
}
