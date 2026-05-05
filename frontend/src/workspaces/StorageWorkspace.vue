<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const showAll = ref(false);
function newEntry() { /* TODO */ }
function refresh()  { /* TODO */ }
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="newEntry">+ Entry</button>
      <label class="tool-toggle">
        <input type="checkbox" v-model="showAll" />
        <span>{{ showAll ? t('common.show') : 'Marked' }}</span>
      </label>
      <button class="tool-btn" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.storage.sidebar_title') }}</h2>
      <p class="muted small">{{ t('workspace.storage.placeholder_side') }}</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">{{ t('workspace.storage.title') }}</h1>
      <p class="muted">{{ t('workspace.storage.placeholder_main') }}</p>
    </template>
  </SplitPane>
</template>
