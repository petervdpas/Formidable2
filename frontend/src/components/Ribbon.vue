<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { WORKSPACES, type WorkspaceId } from "../workspaces";
import { Service as TemplateSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { useTemplates } from "../composables/useTemplates";
import { useProfiles } from "../composables/useProfiles";
import { useConfig } from "../composables/useConfig";
import Icon from "./Icon.vue";

defineProps<{ active: WorkspaceId }>();
const emit = defineEmits<{ (e: "select", id: WorkspaceId): void }>();

const { t } = useI18n();

// Backend owns the rule. Each ribbon dependency maps to a single
// boolean check on its module's Service. We re-fetch when the
// frontend composable list of templates/profiles changes (which
// happens after create/delete) so the ribbon stays in sync without
// us mirroring backend logic on the frontend.
const { filenames: templateFilenames } = useTemplates();
const { profiles } = useProfiles();
const { config } = useConfig();

const hasTemplates = ref(true);
const hasProfiles = ref(true);

async function refreshAvailability() {
  [hasTemplates.value, hasProfiles.value] = await Promise.all([
    TemplateSvc.HasTemplates(),
    ConfigSvc.HasUserProfiles(),
  ]);
}

void refreshAvailability();
watch([templateFilenames, profiles], () => void refreshAvailability(), {
  deep: true,
});

function isDisabled(id: WorkspaceId): boolean {
  if (id === "storage") return !hasTemplates.value;
  if (id === "settings") return !hasProfiles.value;
  // Plugins workspace is gated by the user-config flag (Settings →
  // Advanced → Plugins). Reading config.value reactively means
  // toggling the flag immediately ghosts/un-ghosts the ribbon item.
  if (id === "plugins") return !config.value?.enable_plugins;
  return false;
}

function onClick(id: WorkspaceId) {
  if (isDisabled(id)) return;
  emit("select", id);
}
</script>

<template>
  <nav class="ribbon" :aria-label="t('ribbon.settings')">
    <button
      v-for="w in WORKSPACES"
      :key="w.id"
      class="ribbon-item"
      :class="{ active: w.id === active, disabled: isDisabled(w.id) }"
      :title="t(w.labelKey)"
      :aria-label="t(w.labelKey)"
      :aria-current="w.id === active ? 'page' : undefined"
      :aria-disabled="isDisabled(w.id) ? 'true' : undefined"
      :disabled="isDisabled(w.id)"
      @click="onClick(w.id)"
    >
      <Icon :name="w.iconName" :size="36" />
    </button>
  </nav>
</template>
