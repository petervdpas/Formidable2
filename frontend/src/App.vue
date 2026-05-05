<script setup lang="ts">
import { computed } from "vue";
import Ribbon from "./components/Ribbon.vue";
import Topbar from "./components/Topbar.vue";
import Footer from "./components/Footer.vue";
import ToastContainer from "./components/ToastContainer.vue";
import { WORKSPACES } from "./workspaces";
import { useTheme } from "./composables/useTheme";
import { useActiveWorkspace } from "./composables/useActiveWorkspace";
import { useRestartGate } from "./composables/useRestartGate";

useTheme(); // installs the data-theme attribute reactively

const { active, setActive } = useActiveWorkspace();
const { bootConfig } = useRestartGate();

const activeWorkspace = computed(
  () => WORKSPACES.find((w) => w.id === active.value) ?? WORKSPACES[0],
);

// Don't mount any workspace until we have the persisted config
// snapshot — otherwise SplitPane reads its :initial before
// bootConfig.sidebar_width is populated and locks in the fallback.
const ready = computed(() => bootConfig.value !== null);
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
