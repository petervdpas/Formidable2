<script setup lang="ts">
const menus = ["File", "Theme", "Advanced"];
function save()  { /* TODO */ }
function reset() { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="Settings menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="save">Save</button>
      <button class="tool-btn" @click="reset">Reset</button>
    </div>
  </Teleport>

  <div class="workspace">
    <aside class="workspace-sidebar">
      <h2 class="sidebar-title">Settings</h2>
      <p class="muted small">Categories: General, Theme, Sync, Plugins…</p>
    </aside>
    <section class="workspace-main">
      <h1 class="workspace-heading">Settings</h1>
      <p class="muted">Pick a category from the sidebar.</p>
    </section>
  </div>
</template>
