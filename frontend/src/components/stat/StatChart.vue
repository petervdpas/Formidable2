<script setup lang="ts">
import { computed } from "vue";
import { StatKind, type StatResult } from "./types";
import StatBar from "./StatBar.vue";
import StatStacked from "./StatStacked.vue";
import StatTimeSeries from "./StatTimeSeries.vue";
import StatScalars from "./StatScalars.vue";

// Dispatcher: renders the right chart for a Result. A plugin may pass
// an explicit `type` (its chart envelope's chart.type) to override the
// per-kind default - e.g. force a distribution to a bar today, leaving
// room for a future pie without touching callers. Falls back to the
// Result's kind when type is absent.
const props = defineProps<{
  result: StatResult | null;
  type?: string;
}>();

const component = computed(() => {
  const explicit = props.type ?? "";
  switch (explicit) {
    case "bar":
      return StatBar;
    case "stacked":
    case "crosstab":
      return StatStacked;
    case "line":
    case "timeseries":
      return StatTimeSeries;
    case "scalars":
      return StatScalars;
  }
  switch (props.result?.kind) {
    case StatKind.Crosstab:
      return StatStacked;
    case StatKind.TimeSeries:
      return StatTimeSeries;
    case StatKind.ScalarStats:
      return StatScalars;
    case StatKind.Distribution:
    default:
      return StatBar;
  }
});
</script>

<template>
  <component :is="component" :result="result" />
</template>
