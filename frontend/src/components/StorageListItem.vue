<script setup lang="ts">
import { computed } from "vue";
import { useConfig } from "../composables/useConfig";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { SidebarItem } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const props = defineProps<{
  summary: FormSummary;
  active: boolean;
  expression?: SidebarItem;
}>();

defineEmits<{
  (e: "pick", filename: string): void;
}>();

const { config } = useConfig();

const exprStyle = computed<Record<string, string>>(() => {
  const e = props.expression;
  if (!e) return {};
  const s: Record<string, string> = {};
  if (e.color) s.color = e.color;
  if (e.bg) s.background = e.bg;
  return s;
});
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active }]"
    @click="$emit('pick', summary.filename)"
  >
    <span class="form-list-title">{{ summary.title || summary.filename }}</span>
    <span v-if="config?.development_enable" class="form-list-filename">{{ summary.filename }}</span>
    <span
      v-if="expression"
      class="form-list-expression"
      :class="expression.classes"
      :style="exprStyle"
      :title="expression.error || undefined"
    >{{ expression.text }}</span>
  </li>
</template>
