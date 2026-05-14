<script setup lang="ts">
import { computed } from "vue";
import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  filename: string;
  /** Pre-loaded template, owned by TemplatesWorkspace via useTemplates'
   *  cache (populated by one batched LoadMany on refresh). `null` means
   *  the file is missing or unparseable — the row falls back to the
   *  filename stem. */
  template: Template | null;
  active: boolean;
}>();

defineEmits<{
  (e: "pick", filename: string): void;
}>();

function stemOf(fn: string): string {
  return fn.replace(/\.yaml$/, "");
}

const display = computed<string>(() => {
  const name = props.template?.name?.trim();
  return name && name.length > 0 ? name : stemOf(props.filename);
});
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active: props.active }]"
    :data-filename="props.filename"
    @click="$emit('pick', props.filename)"
  >
    <span class="template-display">{{ display }}</span>
    <span class="template-meta">
      <span class="badge small template-filename">{{ props.filename }}</span>
    </span>
  </li>
</template>
