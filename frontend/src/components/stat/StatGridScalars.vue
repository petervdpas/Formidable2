<script setup lang="ts">
import { computed } from "vue";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { type Grid, scalarValues, fmtNum } from "./grid";

// Rank-0 grid: one card per measure (the measure label is its caption,
// e.g. "count", "avg(amount)"). Drawn as a single SVG (outlined cards
// with a big value + uppercase caption) so the chart is self-contained
// and exports the same as the other chart types. facets/measureIndex
// are accepted for a uniform dispatch signature but unused here.
const props = defineProps<{ grid: Grid; facets?: Facet[]; measureIndex?: number }>();

const PAD = 22;
const CARD_W = 170;
const CARD_H = 72;
const GAP = 12;

const view = computed(() => {
  const cards = scalarValues(props.grid);
  if (cards.length === 0) return null;
  const n = cards.length;
  const W = PAD * 2 + n * CARD_W + (n - 1) * GAP;
  const H = PAD * 2 + CARD_H;
  return {
    W,
    H,
    cards: cards.map((c, i) => ({
      x: PAD + i * (CARD_W + GAP),
      value: fmtNum(c.value),
      label: c.label.toUpperCase(),
    })),
  };
});
</script>

<template>
  <div class="stat-chart">
    <svg
      v-if="view"
      class="stat-svg"
      :viewBox="`0 0 ${view.W} ${view.H}`"
      :style="{ width: `${view.W}px`, maxWidth: '100%', height: 'auto' }"
    >
      <g v-for="(c, i) in view.cards" :key="i">
        <rect :x="c.x" :y="PAD" :width="CARD_W" :height="CARD_H" rx="6" class="stat-svg-card" />
        <text
          :x="c.x + CARD_W / 2"
          :y="PAD + 36"
          text-anchor="middle"
          class="stat-svg-value"
        >{{ c.value }}</text>
        <text
          :x="c.x + CARD_W / 2"
          :y="PAD + 58"
          text-anchor="middle"
          class="stat-svg-caption"
        >{{ c.label }}</text>
      </g>
    </svg>
    <p v-else class="stat-empty">No data.</p>
  </div>
</template>
