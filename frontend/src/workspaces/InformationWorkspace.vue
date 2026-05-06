<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { INFORMATION_CATEGORIES, type InformationCategoryId } from "./information";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const activeId = ref<InformationCategoryId>("about");
const activeCategory = computed(
  () =>
    INFORMATION_CATEGORIES.find((c) => c.id === activeId.value) ??
    INFORMATION_CATEGORIES[0],
);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.information.sidebar_title') }}</h2>
      <ul class="sidebar-list">
        <li
          v-for="c in INFORMATION_CATEGORIES"
          :key="c.id"
          :class="['sidebar-row', { active: c.id === activeId }]"
          @click="activeId = c.id"
        >
          {{ t(c.labelKey) }}
        </li>
      </ul>
    </template>

    <template #main>
      <h1 class="workspace-heading">{{ t(activeCategory.labelKey) }}</h1>
      <component :is="activeCategory.component" />
    </template>
  </SplitPane>
</template>
