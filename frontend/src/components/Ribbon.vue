<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { WORKSPACES, type WorkspaceId } from "../workspaces";
import { useRibbonAvailability } from "../composables/useRibbonAvailability";
import Icon from "./Icon.vue";

defineProps<{ active: WorkspaceId }>();
const emit = defineEmits<{ (e: "select", id: WorkspaceId): void }>();

const { t } = useI18n();
const { isDisabled } = useRibbonAvailability();

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
