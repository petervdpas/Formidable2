<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { WORKSPACES, type WorkspaceId } from "../workspaces";
import Icon from "./Icon.vue";

defineProps<{ active: WorkspaceId }>();
const emit = defineEmits<{ (e: "select", id: WorkspaceId): void }>();

const { t } = useI18n();
</script>

<template>
  <nav class="ribbon" :aria-label="t('ribbon.settings')">
    <button
      v-for="w in WORKSPACES"
      :key="w.id"
      class="ribbon-item"
      :class="{ active: w.id === active }"
      :title="t(w.labelKey)"
      :aria-label="t(w.labelKey)"
      :aria-current="w.id === active ? 'page' : undefined"
      @click="emit('select', w.id)"
    >
      <Icon :name="w.iconName" :size="36" />
    </button>
  </nav>
</template>
