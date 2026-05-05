<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useConfig } from "../composables/useConfig";
import { SETTINGS_CATEGORIES, type SettingsCategoryId } from "./settings";

const { t } = useI18n();

const menus = ["File", "Theme", "Advanced"];

const { config, profileFilename, reload } = useConfig();
const loaded = computed(() => config.value !== null);

const activeId = ref<SettingsCategoryId>("general");
const activeCategory = computed(
  () => SETTINGS_CATEGORIES.find((c) => c.id === activeId.value) ?? SETTINGS_CATEGORIES[0],
);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" :aria-label="t('ribbon.settings')">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <span v-if="profileFilename" class="badge badge-accent">{{ profileFilename }}</span>
      <button class="tool-btn" @click="reload" :title="t('settings.reload_tooltip')">
        {{ t('common.refresh') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="260" :min="180" :max="360">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('settings.title') }}</h2>
      <ul class="sidebar-list">
        <li
          v-for="c in SETTINGS_CATEGORIES"
          :key="c.id"
          :class="['sidebar-list-item', { active: c.id === activeId }]"
          @click="activeId = c.id"
        >
          {{ t(c.labelKey) }}
        </li>
      </ul>
    </template>

    <template #main>
      <h1 class="workspace-heading">{{ t(activeCategory.labelKey) }}</h1>
      <div v-if="loaded">
        <component :is="activeCategory.component" />
      </div>
      <p v-else class="muted">{{ t('settings.loading_config') }}</p>
    </template>
  </SplitPane>
</template>

<style scoped>
.sidebar-list {
    list-style: none;
    padding: 0;
    margin: 0;
}
.sidebar-list-item {
    padding: 6px var(--space-2);
    border-radius: var(--radius-md);
    cursor: pointer;
    color: var(--color-text);
}
.sidebar-list-item:hover { background: var(--list-hover-bg); }
.sidebar-list-item.active {
    background: var(--list-active-bg);
    color: var(--list-active-fg);
    font-weight: 600;
}
</style>
