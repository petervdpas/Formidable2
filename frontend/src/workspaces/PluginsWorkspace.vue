<script setup lang="ts">
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";

const { t } = useI18n();

const menus = ["File", "Registry", "View"];
function install() { /* TODO */ }
function refresh() { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" :aria-label="t('workspace.plugins.title')">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="install">Install…</button>
      <button class="tool-btn" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
  </Teleport>

  <SplitPane>
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
