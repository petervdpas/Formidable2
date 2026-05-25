<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import StatGrid from "../stat/StatGrid.vue";
import type { Grid } from "../stat/grid";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";
import { buildChartSvg } from "../../utils/downloadChartSvg";
import { chooseSaveFile } from "../../composables/useDialog";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// ChartWidget is a passive display wrapper, like ProgressBarWidget and
// StatusMessageWidget: it shows whatever the running plugin pushes via
// formidable.run.chart(spec). Lua steers it - the plugin evaluates a
// statistical object (formidable.statistical.eval returns a rank-N
// Grid) and pushes {type, result}; this widget hands the grid to
// StatGrid (same renderer the Statistics builder uses: rank-1 bar/pie,
// rank-2 heatmap, rank-0 scalar cards). All chart widgets in a form
// share the one global `chart` ref, cleared at the start of every run.
defineProps<{ widget: Widget }>();

const { t } = useI18n();
const toast = useToast();
const { chart } = useGlobalPluginRun();

const grid = computed(() => (chart.value?.result ?? null) as Grid | null);
const type = computed(() => chart.value?.type ?? "");
// Facets travel in the spec so the chart paints categories with their
// authored facet-option colors (facetColorToken in StatGrid).
const facets = computed(() => (chart.value?.facets ?? undefined) as Facet[] | undefined);

const canvas = ref<HTMLElement | null>(null);

async function download(): Promise<void> {
  if (!canvas.value) return;
  const svg = buildChartSvg(canvas.value);
  try {
    const path = await chooseSaveFile("chart.svg", [
      { displayName: "SVG", pattern: "*.svg" },
    ]);
    if (!path) return;
    await SystemSvc.SaveFile(path, svg);
    toast.success("workspace.plugins.chart.download_ok");
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}
</script>

<template>
  <div class="form-widget form-widget-chart">
    <div class="form-widget-chart-head">
      <label v-if="widget.label" class="form-widget-label">{{ widget.label }}</label>
      <button
        v-if="grid"
        type="button"
        class="tool-btn"
        @click="download"
      >
        {{ t('workspace.plugins.chart.download') }}
      </button>
    </div>
    <div class="chart-widget-display">
      <div v-if="grid" ref="canvas" class="chart-widget-canvas">
        <StatGrid :grid="grid" :type="type" :facets="facets" />
      </div>
      <p v-else class="muted small chart-widget-empty">-</p>
    </div>
  </div>
</template>
