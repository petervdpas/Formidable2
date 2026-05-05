import { ref, watch } from "vue";
import { useConfig, type Config } from "./useConfig";

// Restart-only fields — the gate flips on whenever any of these
// diverge from what was loaded at app boot. Add new fields here when
// they become "applies on next launch."
type RestartSnapshot = {
  windowWidth: number;
  windowHeight: number;
  sidebarWidth: number;
};

function snap(cfg: Config | null): RestartSnapshot | null {
  if (!cfg) return null;
  return {
    windowWidth:  cfg.window_bounds?.width  ?? 0,
    windowHeight: cfg.window_bounds?.height ?? 0,
    sidebarWidth: cfg.sidebar_width         ?? 0,
  };
}

function equal(a: RestartSnapshot | null, b: RestartSnapshot | null): boolean {
  if (a == null || b == null) return a === b;
  return a.windowWidth === b.windowWidth
      && a.windowHeight === b.windowHeight
      && a.sidebarWidth === b.sidebarWidth;
}

// bootConfig is the FULL config as it was when the app started. It
// never changes during the session even if the user edits restart-only
// fields — that's the point. Workspaces that depend on restart-only
// values (sidebar width, in/out window size) read from here so the
// session stays internally consistent until the user clicks Apply.
const bootConfig = ref<Config | null>(null);
const needsRestart = ref(false);

const { config } = useConfig();

watch(
  config,
  (cur) => {
    if (cur == null) return;
    if (bootConfig.value == null) {
      bootConfig.value = cur;
      needsRestart.value = false;
      return;
    }
    needsRestart.value = !equal(snap(cur), snap(bootConfig.value));
  },
  { immediate: true, deep: true },
);

export function useRestartGate() {
  return { needsRestart, bootConfig };
}
