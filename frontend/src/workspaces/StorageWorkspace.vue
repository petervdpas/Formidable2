<script setup lang="ts">
import { ref } from "vue";

const menus = ["File", "Edit", "Filter", "Render", "View", "Help"];
const showAll = ref(false);
function newEntry() { /* TODO */ }
function refresh()  { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="Storage menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="newEntry">+ New Entry</button>
      <label class="tool-toggle">
        <input type="checkbox" v-model="showAll" />
        <span>{{ showAll ? "Show All" : "Marked items" }}</span>
      </label>
      <button class="tool-btn" @click="refresh">Refresh</button>
    </div>
  </Teleport>

  <div class="workspace">
    <aside class="workspace-sidebar">
      <h2 class="sidebar-title">Forms</h2>
      <p class="muted small">Form list (per template) goes here.</p>
    </aside>
    <section class="workspace-main">
      <h1 class="workspace-heading">Storage</h1>
      <p class="muted">Pick a form from the sidebar to view or edit its data.</p>
    </section>
  </div>
</template>
