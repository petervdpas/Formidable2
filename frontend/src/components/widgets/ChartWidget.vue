<script setup lang="ts">
import { computed } from "vue";
import StatGrid from "../stat/StatGrid.vue";
import type { Grid } from "../stat/grid";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";

// ChartWidget is a passive display wrapper, like ProgressBarWidget and
// StatusMessageWidget: it shows whatever the running plugin pushes via
// formidable.run.chart(spec). Lua steers it - the plugin evaluates a
// statistical object (formidable.statistical.eval returns a rank-N
// Grid) and pushes {type, result}; this widget hands the grid to
// StatGrid (same renderer the Statistics builder uses: rank-1 bar/pie,
// rank-2 heatmap, rank-0 scalar cards). All chart widgets in a form
// share the one global `chart` ref, cleared at the start of every run.
defineProps<{ widget: Widget }>();

const { chart } = useGlobalPluginRun();

const grid = computed(() => (chart.value?.result ?? null) as Grid | null);
const type = computed(() => chart.value?.type ?? "");
// Facets travel in the spec so the chart paints categories with their
// authored facet-option colors (facetColorToken in StatGrid).
const facets = computed(() => (chart.value?.facets ?? undefined) as Facet[] | undefined);
</script>

<template>
  <div class="form-widget form-widget-chart">
    <label v-if="widget.label" class="form-widget-label">{{ widget.label }}</label>
    <div class="chart-widget-display">
      <div v-if="grid" class="chart-widget-canvas">
        <StatGrid :grid="grid" :type="type" :facets="facets" />
      </div>
      <p v-else class="muted small chart-widget-empty">-</p>
    </div>
  </div>
</template>
