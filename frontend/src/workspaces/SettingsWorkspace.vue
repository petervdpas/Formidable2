<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import AlertDialog from "../components/AlertDialog.vue";
import { useConfig } from "../composables/useConfig";
import { useRestartGate } from "../composables/useRestartGate";
import { useRibbonAvailability } from "../composables/useRibbonAvailability";
// (bootConfig comes from useRestartGate so sidebar width stays frozen
// for the session — same rule as Templates/Storage.)
import { setTopbarMenu } from "../composables/useTopbarMenu";
import { useToast } from "../composables/useToast";
import { SETTINGS_CATEGORIES, type SettingsCategoryId } from "./settings";
import { Service as System } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const { t } = useI18n();

const { config, profileFilename, reload } = useConfig();
const { needsRestart, bootConfig } = useRestartGate();
const { hasProfiles } = useRibbonAvailability();
const toast = useToast();
const loaded = computed(() => config.value !== null);
const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

async function doRefresh() {
  try {
    await reload();
    toast.success("toast.refresh.success");
  } catch (err) {
    toast.error("toast.refresh.error", [String(err)]);
  }
}

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
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: doRefresh,
      },
    ],
  },
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
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('settings.title') }}</h2>
      <ul class="sidebar-list">
        <li
          v-for="c in SETTINGS_CATEGORIES"
          :key="c.id"
          :class="['sidebar-row', { active: c.id === activeId }]"
          @click="activeId = c.id"
        >
          {{ t(c.labelKey) }}
        </li>
      </ul>
    </template>

    <template #main>
      <p
        v-if="!hasProfiles"
        class="workspace-empty"
        v-html="t('settings.no_profile')"
      ></p>
      <template v-else>
        <h1 class="workspace-heading">{{ t(activeCategory.labelKey) }}</h1>
        <div v-if="loaded">
          <component :is="activeCategory.component" />
        </div>
        <p v-else class="muted">{{ t('settings.loading_config') }}</p>
      </template>
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

