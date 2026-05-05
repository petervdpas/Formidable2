<script setup lang="ts">
import { computed } from "vue";
import Ribbon from "./components/Ribbon.vue";
import Topbar from "./components/Topbar.vue";
import Footer from "./components/Footer.vue";
import { WORKSPACES } from "./workspaces";
import { useTheme } from "./composables/useTheme";
import { useActiveWorkspace } from "./composables/useActiveWorkspace";

useTheme(); // installs the data-theme attribute reactively

const { active, setActive } = useActiveWorkspace();
const activeWorkspace = computed(
  () => WORKSPACES.find((w) => w.id === active.value) ?? WORKSPACES[0],
);
</script>

<template>
  <div class="app-shell">
    <Ribbon :active="active" @select="setActive" />
    <Topbar />
    <main class="app-main">
      <component :is="activeWorkspace.component" />
    </main>
    <Footer />
  </div>
</template>
