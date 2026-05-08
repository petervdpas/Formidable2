<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import {
  COLLABORATION_SECTIONS,
  COLLABORATION_BACKEND_VIEWS,
  type CollaborationSectionId,
  type CollaborationBackend,
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

// "none" is the empty-main case — defensive only, since ribbon
// ghosting + App.vue redirect should prevent landing here. Anything
// outside {git, gigot} also collapses to null so we render the
// fallback rather than blow up.
const backend = computed<CollaborationBackend | null>(() => {
  const b = config.value?.remote_backend;
  return b === "git" || b === "gigot" ? b : null;
});

const backendView = computed(() => (backend.value ? COLLABORATION_BACKEND_VIEWS[backend.value] : null));
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
        v-if="!backendView"
        class="workspace-empty"
        v-html="t('workspace.collaboration.no_backend')"
      ></p>
      <template v-else>
        <h1 class="workspace-heading">{{ t(activeSection.labelKey) }}</h1>
        <component :is="backendView" />
      </template>
    </template>
  </SplitPane>
</template>
