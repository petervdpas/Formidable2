<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

function install() { /* TODO */ }
function refresh() { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="install">Install…</button>
      <button class="tool-btn" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.plugins.sidebar_title') }}</h2>
      <p class="muted small">{{ t('workspace.plugins.placeholder_side') }}</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">{{ t('workspace.plugins.title') }}</h1>
      <p class="muted">{{ t('workspace.plugins.placeholder_main') }}</p>
    </template>
  </SplitPane>
</template>
