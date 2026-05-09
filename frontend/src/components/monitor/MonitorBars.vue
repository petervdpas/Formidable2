<script setup lang="ts">
import { computed } from "vue";
import type { Result } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/monitor/models";

const props = withDefaults(
  defineProps<{
    result: Result | null;
    width?: number;
    height?: number;
  }>(),
  { width: 520, height: 200 },
);

const PAD_LEFT = 100;
const PAD_RIGHT = 36;
const PAD_TOP = 12;
const BAR_HEIGHT = 18;
const BAR_GAP = 10;

const view = computed(() => {
  const series = props.result?.series ?? [];
  if (series.length === 0) return null;

  const totals = series.map((s) => ({
    label: formatKey(s.key),
    value: s.total ?? 0,
  }));

  const max = Math.max(1, ...totals.map((t) => t.value));
  const barAreaW = props.width - PAD_LEFT - PAD_RIGHT;

  const rows = totals.map((t, i) => ({
    y: PAD_TOP + i * (BAR_HEIGHT + BAR_GAP),
    width: (t.value / max) * barAreaW,
    label: t.label,
    value: t.value,
  }));

  // Auto-grow height to fit all bars.
  const minHeight =
    PAD_TOP + rows.length * (BAR_HEIGHT + BAR_GAP) + 4;
  const height = Math.max(props.height, minHeight);

  return { rows, height };
});

function formatKey(key: { [k: string]: string | undefined }): string {
  const parts = Object.entries(key)
    .filter(([_, v]) => v !== undefined && v !== "")
    .map(([_, v]) => v as string);
  return parts.length === 0 ? "all" : parts.join(" / ");
}
</script>

<template>
  <div class="monitor-chart">
    <svg
      v-if="view"
      class="monitor-svg"
      :viewBox="`0 0 ${props.width} ${view.height}`"
      preserveAspectRatio="none"
    >
      <g class="monitor-bars">
        <g v-for="(row, i) in view.rows" :key="`bar-${i}`">
          <text
            :x="96"
            :y="row.y + 13"
            text-anchor="end"
            class="monitor-bar-label"
          >{{ row.label }}</text>
          <rect
            :x="100"
            :y="row.y"
            :width="row.width"
            :height="18"
            class="monitor-bar"
          />
          <text
            :x="100 + row.width + 6"
            :y="row.y + 13"
            class="monitor-bar-value"
          >{{ row.value }}</text>
        </g>
      </g>
    </svg>
    <p v-else class="monitor-empty">No data.</p>
  </div>
</template>
