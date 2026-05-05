<script setup lang="ts">
import { computed, watch } from "vue";
import Ribbon from "./components/Ribbon.vue";
import Topbar from "./components/Topbar.vue";
import Footer from "./components/Footer.vue";
import ToastContainer from "./components/ToastContainer.vue";
import { WORKSPACES, type WorkspaceId } from "./workspaces";
import { useTheme } from "./composables/useTheme";
import { useActiveWorkspace } from "./composables/useActiveWorkspace";
import { useRestartGate } from "./composables/useRestartGate";
import { useConfig } from "./composables/useConfig";

useTheme(); // installs the data-theme attribute reactively

const { active, setActive } = useActiveWorkspace();
const { bootConfig } = useRestartGate();
const { update } = useConfig();

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
  </div>
</template>
