<script setup lang="ts">
import { computed } from "vue";
import StatChart from "../stat/StatChart.vue";
import type { StatResult } from "../stat/types";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";

// ChartWidget is a passive display wrapper, like ProgressBarWidget and
// StatusMessageWidget: it shows whatever the running plugin pushes via
// formidable.run.chart(spec). Lua steers it - the plugin reads the
// form's object/shape fields, evaluates a statistical object, and
// pushes {type, result}; this widget renders the result with StatChart.
// All chart widgets in a form share the one global `chart` ref, cleared
// at the start of every run.
defineProps<{ widget: Widget }>();

const { chart } = useGlobalPluginRun();

const result = computed(() => (chart.value?.result ?? null) as StatResult | null);
const type = computed(() => chart.value?.type ?? "");
</script>

<template>
  <div class="form-widget form-widget-chart">
    <label v-if="widget.label" class="form-widget-label">{{ widget.label }}</label>
    <div class="chart-widget-display">
      <StatChart v-if="result" :result="result" :type="type" />
      <p v-else class="muted small chart-widget-empty">-</p>
    </div>
  </div>
</template>
