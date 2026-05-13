<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const props = defineProps<{
  collapsed: boolean[];
}>();

defineEmits<{
  (e: "expand-all"): void;
  (e: "collapse-all"): void;
}>();

const hasItems = computed(() => props.collapsed.length > 0);
const allCollapsed = computed(() => hasItems.value && props.collapsed.every((c) => c));
const allExpanded = computed(() => hasItems.value && props.collapsed.every((c) => !c));
</script>

<template>
  <div class="form-loop-bulk-toggle" role="group">
    <button
      type="button"
      class="btn-ghost-icon btn-sm"
      :disabled="!hasItems || allExpanded"
      :title="t('workspace.storage.field.loop.expand_all')"
      :aria-label="t('workspace.storage.field.loop.expand_all')"
      @click="$emit('expand-all')"
    >
      <i class="fa-solid fa-chevron-down" aria-hidden="true"></i>
    </button>
    <button
      type="button"
      class="btn-ghost-icon btn-sm"
      :disabled="!hasItems || allCollapsed"
      :title="t('workspace.storage.field.loop.collapse_all')"
      :aria-label="t('workspace.storage.field.loop.collapse_all')"
      @click="$emit('collapse-all')"
    >
      <i class="fa-solid fa-chevron-up" aria-hidden="true"></i>
    </button>
  </div>
</template>
