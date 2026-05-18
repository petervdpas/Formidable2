<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from "vue";
import { Events } from "@wailsio/runtime";
import Ribbon from "./components/Ribbon.vue";
import Topbar from "./components/Topbar.vue";
import Footer from "./components/Footer.vue";
import ToastContainer from "./components/ToastContainer.vue";
import PluginRunDialog from "./components/PluginRunDialog.vue";
import { WORKSPACES, type WorkspaceId } from "./workspaces";
import { useTheme } from "./composables/useTheme";
import { useActiveWorkspace } from "./composables/useActiveWorkspace";
import { useRestartGate } from "./composables/useRestartGate";
import { useConfig } from "./composables/useConfig";
import { useRibbonAvailability } from "./composables/useRibbonAvailability";

useTheme(); // installs the data-theme attribute reactively

const { active, setActive } = useActiveWorkspace();
const { bootConfig } = useRestartGate();
const { config, update, reload } = useConfig();
const { hasTemplates, hasProfiles, isDisabled, fallbackFor } =
  useRibbonAvailability();

// Suppress the WebView's default context menu (back/forward/reload/inspect)
// unless development_enable is on. Reads the live config at fire time so
// toggling Development Mode in Settings takes effect without a reload.
function onContextMenu(e: MouseEvent) {
  if (!config.value?.development_enable) e.preventDefault();
}

const activeWorkspace = computed(
  () => WORKSPACES.find((w) => w.id === active.value) ?? WORKSPACES[0],
);

// Don't mount any workspace until we have the persisted config
// snapshot — otherwise SplitPane reads its :initial before
// bootConfig.sidebar_width is populated and locks in the fallback.
const ready = computed(() => bootConfig.value !== null);

// Restore the persisted ribbon as soon as bootConfig lands, then
// persist any subsequent change back to disk. We use a one-shot
// watcher for the restore so it doesn't fight a user who clicked a
// different ribbon between mount and config load.
const validIds = new Set<string>(WORKSPACES.map((w) => w.id));
let restored = false;

watch(
  bootConfig,
  (cfg) => {
    if (!cfg || restored) return;
    restored = true;
    const want = cfg.context_ribbon;
    if (want && validIds.has(want) && want !== active.value) {
      setActive(want as WorkspaceId);
    }
  },
  { immediate: true },
);

// Persist ribbon changes. Skip the very first run (it's just the
// initial value, possibly equal to what's already on disk).
watch(active, (id) => {
  if (!restored) return;
  void update({ context_ribbon: id });
});

// Redirect away from a disabled workspace. Triggers in two scenarios:
//   1. At boot, after availability resolves, if the persisted ribbon
//      points at a workspace that's currently unavailable (e.g.
//      Storage was last active but the user has zero templates).
//   2. During the session, if the active workspace becomes disabled
//      (e.g. user deletes the last template while on Storage).
//
// We watch the booleans (hasTemplates/hasProfiles) plus active so the
// effect re-runs whenever any of them changes. If a fallback is also
// disabled (shouldn't happen with current rules, but be safe), we
// stop redirecting rather than loop.
watch(
  [active, hasTemplates, hasProfiles],
  () => {
    if (!isDisabled(active.value)) return;
    const fb = fallbackFor(active.value);
    if (!fb || fb === active.value || isDisabled(fb)) return;
    setActive(fb);
  },
  { immediate: true },
);

// nav:changed — backend (nav.Manager) emits this after a successful
// formidable:// navigation. Backend already wrote the new selection
// to config; we refresh the local cache so workspace watchers fire,
// then flip the active workspace to Storage. The event payload is
// { template, datafile, fragment? }.
let unsubNav: (() => void) | null = null;
onMounted(() => {
  unsubNav = Events.On("nav:changed", () => {
    void reload();
    if (active.value !== "storage") {
      setActive("storage");
    }
  });
  window.addEventListener("contextmenu", onContextMenu);
});
onBeforeUnmount(() => {
  unsubNav?.();
  unsubNav = null;
  window.removeEventListener("contextmenu", onContextMenu);
});
</script>

<template>
  <div class="app-shell">
    <Ribbon :active="active" @select="setActive" />
    <Topbar />
    <main class="app-main">
      <component v-if="ready" :is="activeWorkspace.component" />
    </main>
    <Footer />
    <ToastContainer />
    <PluginRunDialog />
  </div>
</template>
