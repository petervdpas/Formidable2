<script setup lang="ts">
const menus = ["File", "Edit", "Sync", "View"];
function newProfile() { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="Profiles menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="newProfile">+ New Profile</button>
    </div>
  </Teleport>

  <div class="workspace">
    <aside class="workspace-sidebar">
      <h2 class="sidebar-title">Profiles</h2>
      <p class="muted small">Profile list goes here.</p>
    </aside>
    <section class="workspace-main">
      <h1 class="workspace-heading">Profiles</h1>
      <p class="muted">Switch the active profile or edit profile-scoped settings.</p>
    </section>
  </div>
</template>
