<script setup lang="ts">
import { computed, ref } from "vue";
import Ribbon from "./components/Ribbon.vue";
import Topbar from "./components/Topbar.vue";
import Footer from "./components/Footer.vue";
import { WORKSPACES, type WorkspaceId } from "./workspaces";

const active = ref<WorkspaceId>("templates");
const activeWorkspace = computed(
  () => WORKSPACES.find((w) => w.id === active.value) ?? WORKSPACES[0],
);
</script>

<template>
  <div class="app-shell">
    <Ribbon :active="active" @select="active = $event" />
    <Topbar />
    <main class="app-main">
      <component :is="activeWorkspace.component" />
    </main>
    <Footer />
  </div>
</template>
