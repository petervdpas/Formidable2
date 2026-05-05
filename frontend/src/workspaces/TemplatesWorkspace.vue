<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

// Sidebar width is "applies on next launch" — read from the boot
// snapshot so editing it in Settings doesn't change the layout
// mid-session (that would lie about what the Apply button means).
const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

function newTemplate()    { /* TODO */ }
function importTemplate() { /* TODO */ }
function refreshList()    { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
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
