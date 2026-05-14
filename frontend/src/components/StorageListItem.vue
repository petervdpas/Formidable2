<script setup lang="ts">
import { computed } from "vue";
import { useConfig } from "../composables/useConfig";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Result as ExpressionResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const props = defineProps<{
  summary: FormSummary;
  /** Pre-evaluated sidebar sub-label for this row, owned by the parent
   *  StorageWorkspace's `sidebarItems` map. The workspace populates the
   *  map via one batched EvaluateListMany on list load / Refresh,
   *  and updates the single key via EvaluateListOne after a save —
   *  no per-row IPC on mount. `null` means "no sub-label to show". */
  expression: ExpressionResult | null;
  active: boolean;
  flagColor?: string;
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

const flagState = computed(() => props.summary.meta?.flag_state ?? "");
const flagged = computed(() => !!props.summary.meta?.flagged);
const showFlag = computed(() => flagState.value !== "" || flagged.value);
const flagIconClass = computed(() => {
  if (props.flagColor) return `expr-text-${props.flagColor}`;
  return "flag-picker-empty";
});
const flagTitle = computed(() => flagState.value || (flagged.value ? "✓" : ""));
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active }]"
    :data-filename="summary.filename"
    @click="$emit('pick', summary.filename)"
  >
    <span class="form-list-title-row">
      <span class="form-list-title">{{ summary.title || summary.filename }}</span>
      <i
        v-if="showFlag"
        class="fa-solid fa-flag form-list-flag"
        :class="flagIconClass"
        :title="flagTitle"
        aria-hidden="true"
      ></i>
    </span>
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
