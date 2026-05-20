<script setup lang="ts">
import { computed } from "vue";
import { useConfig } from "../composables/useConfig";
import FacetIcon from "./FacetIcon.vue";
import type { FormSummary } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Result as ExpressionResult } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/expression";

const props = defineProps<{
  summary: FormSummary;
  /** Pre-evaluated sidebar sub-label for this row, owned by the parent
   *  StorageWorkspace's `sidebarItems` map. */
  expression: ExpressionResult | null;
  active: boolean;
  /** Template facets — drives which icons appear per row. */
  facets: Facet[];
}>();

const emit = defineEmits<{
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

type FacetChip = {
  key: string;
  icon: string;
  colorClass: string;
  title: string;
};

const MAX_INLINE_CHIPS = 4;

const allActiveChips = computed<FacetChip[]>(() => {
  const state = props.summary.meta?.facets;
  if (!state) return [];
  const out: FacetChip[] = [];
  for (const f of props.facets) {
    const s = state[f.key];
    if (!s || !s.set) continue;
    const opt = f.options.find((o) => o.label === s.selected);
    out.push({
      key: f.key,
      icon: f.icon,
      colorClass: opt ? `expr-text-${opt.color}` : "facet-picker-empty",
      title: opt ? `${f.key}: ${opt.label}` : f.key,
    });
  }
  return out;
});

// Visual rule: render each facet's own icon when ≤ MAX_INLINE_CHIPS are
// active on a row; collapse to a single generic flag icon otherwise so
// the sidebar stays visually balanced.
const collapsed = computed(() => allActiveChips.value.length > MAX_INLINE_CHIPS);

const displayChips = computed<FacetChip[]>(() => {
  if (!collapsed.value) return allActiveChips.value;
  const keys = allActiveChips.value.map((c) => c.key).join(", ");
  return [
    {
      key: "__collapsed",
      icon: "fa-flag",
      colorClass: "facet-picker-empty",
      title: keys,
    },
  ];
});
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active }]"
    :data-filename="summary.filename"
    @click="emit('pick', summary.filename)"
  >
    <span class="form-list-title-row">
      <span class="form-list-title">{{ summary.title || summary.filename }}</span>
      <span v-if="displayChips.length > 0" class="form-list-facets">
        <FacetIcon
          v-for="c in displayChips"
          :key="c.key"
          :icon="c.icon"
          :class="[c.colorClass, 'form-list-facet']"
          :title="c.title"
        />
      </span>
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
