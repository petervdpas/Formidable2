<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useConfig } from "../composables/useConfig";

const { t } = useI18n();
const { config } = useConfig();

// Sidebar width comes from settings; the SplitPane mounts fresh each
// time the user enters this workspace, so the current value is always
// honored. Drag stays session-only (doesn't write back).
const sidebarWidth = computed(() => config.value?.sidebar_width || 280);

const menus = ["File", "Edit", "Fields", "Validate", "View", "Help"];
function newTemplate()    { /* TODO */ }
function importTemplate() { /* TODO */ }
function refreshList()    { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" :aria-label="t('workspace.templates.title')">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="newTemplate">+ {{ t('workspace.templates.title') }}</button>
      <button class="tool-btn" @click="importTemplate">{{ t('common.import') }}</button>
      <button class="tool-btn" @click="refreshList">{{ t('common.refresh') }}</button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.templates.sidebar_title') }}</h2>
      <p class="muted small">{{ t('workspace.templates.placeholder_side') }}</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">{{ t('workspace.templates.title') }}</h1>
      <p class="muted">{{ t('workspace.templates.placeholder_main') }}</p>
    </template>
  </SplitPane>
</template>
