<script setup lang="ts">
import SplitPane from "../components/SplitPane.vue";

const menus = ["File", "Edit", "Fields", "Validate", "View", "Help"];
function newTemplate()    { /* TODO */ }
function importTemplate() { /* TODO */ }
function refreshList()    { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="Templates menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="newTemplate">+ New Template</button>
      <button class="tool-btn" @click="importTemplate">Import</button>
      <button class="tool-btn" @click="refreshList">Refresh</button>
    </div>
  </Teleport>

  <SplitPane>
    <template #sidebar>
      <h2 class="sidebar-title">Templates</h2>
      <p class="muted small">Template list goes here.</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">Templates</h1>
      <p class="muted">Select a template from the sidebar to edit it.</p>
    </template>
  </SplitPane>
</template>
