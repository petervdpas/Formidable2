<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, type CompositeGrid, gridRank, isCompositeGrid } from "./grid";
import StatGridScalars from "./StatGridScalars.vue";
import StatGridBar from "./StatGridBar.vue";
import StatGridPie from "./StatGridPie.vue";
import StatGridHeatmap from "./StatGridHeatmap.vue";
import StatGridSunburst from "./StatGridSunburst.vue";

// Renders a rank-N Grid or a CompositeGrid. The statistic itself is
// presentation-free, so `type` is the viewer's choice: rank-1 defaults to
// "bar" (or "pie"), rank-2 to "heatmap", rank-0 to scalar cards, and a
// composite (hop route) draws as a sunburst. measureIndex selects which
// measure layer to draw; facets colors categories by facet option.
const props = withDefaults(
  defineProps<{ grid: Grid | CompositeGrid | null; type?: string; measureIndex?: number; facets?: Facet[] }>(),
  { measureIndex: 0 },
);

const composite = computed(() => (isCompositeGrid(props.grid) ? props.grid : null));
const plainGrid = computed(() => (composite.value ? null : (props.grid as Grid | null)));
const rank = computed(() => gridRank(plainGrid.value));
const component = computed(() => {
  if (composite.value) return StatGridSunburst;
  if (!plainGrid.value) return null;
  if (rank.value === 0) return StatGridScalars;
  if (rank.value === 1) return props.type === "pie" ? StatGridPie : StatGridBar;
  if (rank.value === 2) return StatGridHeatmap;
  return null; // rank > 2: no built-in renderer yet
});
</script>

<template>
  <StatGridSunburst v-if="composite" :composite="composite" :facets="facets" />
  <component
    :is="component"
    v-else-if="component && plainGrid"
    :grid="plainGrid"
    :measure-index="measureIndex"
    :facets="facets"
  />
  <p v-else class="stat-empty">Nothing to render.</p>
</template>
