<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useConfig } from "../composables/useConfig";
import { Service as ExpressionSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { SidebarItem } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const props = defineProps<{
  summary: FormSummary;
  templateFilename: string;
  active: boolean;
  /** Color token for the flag icon. Resolved by the parent from the
   *  active template's flag_definitions. Undefined when the form has
   *  no flag_state OR the state references a definition that no longer
   *  exists; in both cases we fall back to a muted gray when the legacy
   *  `flagged` bool is true, otherwise the icon is hidden. */
  flagColor?: string;
}>();

defineEmits<{
  (e: "pick", filename: string): void;
}>();

const { config } = useConfig();

// Self-serving sidebar expression: each row asks the backend for its
// own SidebarItem instead of being fed from a parent-owned Map. On
// save, the workspace calls refresh() on the matching item's ref —
// the rest of the list stays put, scroll position included.
const expression = ref<SidebarItem | null>(null);

async function loadExpression(): Promise<void> {
  if (!config.value?.use_expressions || !props.templateFilename || !props.summary.filename) {
    expression.value = null;
    return;
  }
  try {
    const item = await ExpressionSvc.EvaluateSidebarOne(
      props.templateFilename,
      props.summary.filename,
    );
    expression.value = item?.filename ? item : null;
  } catch {
    // ErrNoExpression + any failure mean "no sub-label" — same posture
    // as the previous bulk path swallowed.
    expression.value = null;
  }
}

onMounted(loadExpression);
watch(
  () => [props.templateFilename, props.summary.filename, config.value?.use_expressions] as const,
  loadExpression,
);

defineExpose({ refresh: loadExpression });

const exprStyle = computed<Record<string, string>>(() => {
  const e = expression.value;
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
