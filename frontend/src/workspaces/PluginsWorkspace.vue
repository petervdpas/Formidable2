<script setup lang="ts">
import SplitPane from "../components/SplitPane.vue";

const menus = ["File", "Registry", "View"];
function install() { /* TODO */ }
function refresh() { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="Plugins menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="install">Install…</button>
      <button class="tool-btn" @click="refresh">Refresh</button>
    </div>
  </Teleport>

  <SplitPane>
    <template #sidebar>
      <h2 class="sidebar-title">Plugins</h2>
      <p class="muted small">Installed plugin list goes here.</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">Plugins</h1>
      <p class="muted">Manage frontend plugins and their settings.</p>
    </template>
  </SplitPane>
</template>
