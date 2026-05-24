<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, gridRank } from "./grid";
import StatGridScalars from "./StatGridScalars.vue";
import StatGridBar from "./StatGridBar.vue";
import StatGridPie from "./StatGridPie.vue";
import StatGridHeatmap from "./StatGridHeatmap.vue";

// Renders a rank-N Grid. The statistic itself is presentation-free, so
// `type` is the viewer's choice: rank-1 defaults to "bar" (or "pie"),
// rank-2 to "heatmap", rank-0 to scalar cards. measureIndex selects which
// measure layer to draw for bar/pie/heatmap (cards show all measures).
// facets lets the rank-1 renderers color categories by facet option.
const props = withDefaults(
  defineProps<{ grid: Grid | null; type?: string; measureIndex?: number; facets?: Facet[] }>(),
  { measureIndex: 0 },
);

const rank = computed(() => gridRank(props.grid));
const component = computed(() => {
  if (!props.grid) return null;
  if (rank.value === 0) return StatGridScalars;
  if (rank.value === 1) return props.type === "pie" ? StatGridPie : StatGridBar;
  if (rank.value === 2) return StatGridHeatmap;
  return null; // rank > 2: no built-in renderer yet
});
</script>

<template>
  <component
    :is="component"
    v-if="component && grid"
    :grid="grid"
    :measure-index="measureIndex"
    :facets="facets"
  />
  <p v-else class="stat-empty">Nothing to render.</p>
</template>
