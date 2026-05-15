<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { useConfig } from "../composables/useConfig";
import { useInformationSection } from "../composables/useInformationSection";
import { INFORMATION_CATEGORIES } from "./information";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const { config } = useConfig();
const { active: activeId, setActive } = useInformationSection();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

// Filter the static category list against the current config snapshot
// so dev/logging-only entries (e.g. Logging) don't appear when
// disabled. Reactive: toggling the underlying flag in Settings adds
// or drops the entry without a reload.
const visibleCategories = computed(() =>
  INFORMATION_CATEGORIES.filter((c) => !c.available || c.available(config.value)),
);

// If the active entry becomes unavailable (user just turned the
// feature off while sitting on it), bounce to the first visible one.
watch(visibleCategories, (list) => {
  if (!list.find((c) => c.id === activeId.value)) {
    setActive(list[0]?.id ?? "about");
  }
});

const activeCategory = computed(
  () =>
    visibleCategories.value.find((c) => c.id === activeId.value) ??
    visibleCategories.value[0],
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
          v-for="c in visibleCategories"
          :key="c.id"
          :class="['sidebar-row', { active: c.id === activeId }]"
          @click="setActive(c.id)"
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
