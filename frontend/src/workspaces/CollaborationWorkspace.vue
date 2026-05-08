<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import {
  COLLABORATION_SECTIONS,
  type CollaborationSectionId,
  type CollaborationBackend,
} from "./collaboration";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config } = useConfig();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const backend = computed<CollaborationBackend | null>(() => {
  const b = config.value?.remote_backend;
  return b === "git" || b === "gigot" ? b : null;
});

// Sidebar shows backend-agnostic rows (no `backend` tag) plus rows
// matching the active backend. Switching backend mid-session
// reactively re-filters; the watcher below corrects activeId if
// it points at a now-hidden row.
const visibleSections = computed(() =>
  COLLABORATION_SECTIONS.filter(
    (s) => !s.backend || s.backend === backend.value,
  ),
);

const activeId = ref<CollaborationSectionId>("current-service");
const activeSection = computed(
  () =>
    visibleSections.value.find((s) => s.id === activeId.value) ??
    visibleSections.value[0],
);

watch(visibleSections, (sections) => {
  if (!sections.find((s) => s.id === activeId.value)) {
    activeId.value = sections[0]?.id ?? "current-service";
  }
});

// Defensive empty-main: ribbon ghosting + App.vue redirect should
// keep "none" out of reach, but render a clear fallback if it ever
// happens (deleted config, race condition, manual nav).
const hasBackend = computed(() => backend.value !== null);
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
          v-for="s in visibleSections"
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
