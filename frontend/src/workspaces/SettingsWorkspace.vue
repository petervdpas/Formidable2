<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import AlertDialog from "../components/AlertDialog.vue";
import { useConfig } from "../composables/useConfig";
import { useRestartGate } from "../composables/useRestartGate";
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { SETTINGS_CATEGORIES, type SettingsCategoryId } from "./settings";
import { Service as System } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const { t } = useI18n();

const { config, profileFilename, reload } = useConfig();
const { needsRestart } = useRestartGate();
const loaded = computed(() => config.value !== null);

const activeId = ref<SettingsCategoryId>("general");
const activeCategory = computed(
  () => SETTINGS_CATEGORIES.find((c) => c.id === activeId.value) ?? SETTINGS_CATEGORIES[0],
);

// Apply (= restart) flow: confirm dialog → backend Restart() → process
// closes itself in ~200 ms. On error, surface in an AlertDialog so the
// user knows the click did something.
const restartConfirmOpen = ref(false);
const restartErrorOpen = ref(false);
const restartErrorMessage = ref("");

function openRestartConfirm() {
  restartConfirmOpen.value = true;
}

async function confirmRestart() {
  restartConfirmOpen.value = false;
  try {
    await System.Restart();
  } catch (err) {
    restartErrorMessage.value = t("settings.apply_error", [String(err)]);
    restartErrorOpen.value = true;
  }
}

// Topbar menu — declarative. The getter is reactive: needsRestart
// flipping toggles the menu item's `disabled` automatically.
setTopbarMenu(() => [
  {
    type: "group",
    id: "apply",
    labelKey: "settings.menu.apply",
    items: [
      {
        id: "apply-restart",
        labelKey: "settings.apply_changes",
        disabled: !needsRestart.value,
        onClick: openRestartConfirm,
      },
    ],
  },
]);
</script>

<template>
  <Teleport defer to="#topbar-content">
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

  <ConfirmDialog
    :open="restartConfirmOpen"
    :title="t('settings.apply_confirm.title')"
    :message="t('settings.apply_confirm.body')"
    :confirm-label="t('settings.apply_confirm.button')"
    :cancel-label="t('common.cancel')"
    @cancel="restartConfirmOpen = false"
    @confirm="confirmRestart"
  />

  <AlertDialog
    :open="restartErrorOpen"
    :title="t('common.error_title')"
    :message="restartErrorMessage"
    variant="danger"
    @close="restartErrorOpen = false"
  />
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
