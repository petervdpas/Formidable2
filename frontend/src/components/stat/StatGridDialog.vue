<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import { SelectField } from "../fields";
import StatGrid from "./StatGrid.vue";
import { type Grid, type CompositeGrid, gridRank, isCompositeGrid } from "./grid";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Glance-and-close dialog for one evaluated statistic. Rank decides the
// available chart types (the statistic carries no presentation), and a
// measure picker appears when the grid has more than one measure. facets
// lets rank-1 charts color categories by their facet option colors. A
// composite (hop route) draws as a sunburst with no chart/measure controls.
const props = defineProps<{
  open: boolean;
  title?: string;
  grid: Grid | CompositeGrid | null;
  facets?: Facet[];
}>();

const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();

const composite = computed(() => isCompositeGrid(props.grid));
const plainGrid = computed(() => (composite.value ? null : (props.grid as Grid | null)));
const rank = computed(() => gridRank(plainGrid.value));

const chartType = ref<string>("");
const measureIndex = ref<number>(0);

const chartTypeOptions = computed(() => {
  if (rank.value === 1) {
    return [
      { value: "bar", label: t("workspace.templates.stat_view.chart.bar") },
      { value: "pie", label: t("workspace.templates.stat_view.chart.pie") },
    ];
  }
  return [];
});

const measureOptions = computed(() =>
  (plainGrid.value?.measures ?? []).map((m, i) => ({ value: String(i), label: m })),
);

// Reset selectors whenever a new grid arrives.
watch(
  () => [props.open, props.grid] as const,
  () => {
    chartType.value = rank.value === 1 ? "bar" : "";
    measureIndex.value = 0;
  },
);
</script>

<template>
  <Modal
    :open="props.open"
    :title="props.title || t('workspace.templates.stat_view.title')"
    width="640px"
    @close="emit('close')"
  >
    <div class="stat-view-controls">
      <SelectField
        v-if="chartTypeOptions.length > 1"
        v-model="chartType"
        :options="chartTypeOptions"
      />
      <SelectField
        v-if="measureOptions.length > 1 && rank > 0"
        :model-value="String(measureIndex)"
        :options="measureOptions"
        @update:model-value="(v: string) => (measureIndex = Number(v))"
      />
    </div>

    <StatGrid :grid="grid" :type="chartType" :measure-index="measureIndex" :facets="facets" />

    <p v-if="plainGrid && plainGrid.total >= 0" class="muted small stat-view-total">
      {{ t('workspace.templates.stat_view.total', [plainGrid.total]) }}
    </p>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.close') }}
      </button>
    </template>
  </Modal>
</template>
