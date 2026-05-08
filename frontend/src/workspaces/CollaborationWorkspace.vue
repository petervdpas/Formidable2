<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import {
  COLLABORATION_SECTIONS,
  type CollaborationSectionId,
} from "./collaboration";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config } = useConfig();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const activeId = ref<CollaborationSectionId>("current-service");
const activeSection = computed(
  () =>
    COLLABORATION_SECTIONS.find((s) => s.id === activeId.value) ??
    COLLABORATION_SECTIONS[0],
);

// Defensive empty-main: ribbon ghosting + App.vue redirect should
// keep "none" out of reach, but render a clear fallback if it ever
// happens (deleted config, race condition, manual nav).
const hasBackend = computed(() => {
  const b = config.value?.remote_backend;
  return b === "git" || b === "gigot";
});
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.collaboration.sidebar_title') }}</h2>
      <ul class="sidebar-list">
        <li
          v-for="s in COLLABORATION_SECTIONS"
          :key="s.id"
          :class="['sidebar-row', { active: s.id === activeId }]"
          @click="activeId = s.id"
        >
          {{ t(s.labelKey) }}
        </li>
      </ul>
    </template>

    <template #main>
      <p
        v-if="!hasBackend"
        class="workspace-empty"
        v-html="t('workspace.collaboration.no_backend')"
      ></p>
      <template v-else>
        <h1 class="workspace-heading">{{ t(activeSection.labelKey) }}</h1>
        <component :is="activeSection.component" />
      </template>
    </template>
  </SplitPane>
</template>
