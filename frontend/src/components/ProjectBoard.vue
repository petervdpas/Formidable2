<script setup lang="ts">
// Plan-board grid (presentational): draws a structured board (render.Board) as a
// time (X) by resource (Y) grid. Backend does the date math; this lays it out.
// The X axis is windowed (a few columns at a time, move left/right); the Y axis
// (resource gutter) stays visible. Clicking a bar emits its source entry index.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { Board, BoardBar } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";

const props = withDefaults(
  defineProps<{
    board: Board | null;
    /** How many time columns to show at once (default 4). */
    windowSize?: number;
  }>(),
  { windowSize: 4 },
);
const emit = defineEmits<{ (e: "edit", index: number): void }>();

const { t } = useI18n();

const allTicks = computed(() => props.board?.ticks ?? []);
const resources = computed(() => props.board?.resources ?? []);
const hasAxis = computed(() => allTicks.value.length > 0 && resources.value.length > 0);

// X-axis window: show windowSize columns starting at windowStart.
const windowStart = ref(0);
const maxStart = computed(() => Math.max(0, allTicks.value.length - props.windowSize));
watch(maxStart, (m) => {
  if (windowStart.value > m) windowStart.value = m;
});
const visibleTicks = computed(() =>
  allTicks.value.slice(windowStart.value, windowStart.value + props.windowSize),
);
const canPrev = computed(() => windowStart.value > 0);
const canNext = computed(() => windowStart.value < maxStart.value);
function prev() {
  if (canPrev.value) windowStart.value -= 1;
}
function next() {
  if (canNext.value) windowStart.value += 1;
}

function barsFor(resourceValue: string): BoardBar[] {
  return (props.board?.bars ?? []).filter((b) => b.resource === resourceValue);
}

function cellColumn(i: number): string {
  return `${i + 1} / ${i + 2}`;
}

// Day-precise placement: bars are positioned by their actual dates within the
// visible window's date range (not snapped to whole tick columns), so a bar that
// starts mid-week starts mid-column.
function isoDays(a: string, b: string): number {
  const da = new Date(a + "T00:00:00");
  const db = new Date(b + "T00:00:00");
  return Math.round((db.getTime() - da.getTime()) / 86400000);
}
const winStart = computed(() => visibleTicks.value[0]?.start ?? "");
const winEnd = computed(
  () => visibleTicks.value[visibleTicks.value.length - 1]?.end ?? "",
);
const winDays = computed(() =>
  winStart.value && winEnd.value ? Math.max(1, isoDays(winStart.value, winEnd.value)) : 1,
);

// Left/width percentages within the lane, clamped to the window, or null when the
// bar falls entirely outside it. End dates are inclusive, so the bar runs to the
// end of the end day (+1).
function barStyle(bar: BoardBar): { left: string; width: string } | null {
  if (!winStart.value) return null;
  const endInclusive = bar.end && bar.end >= bar.start ? bar.end : bar.start;
  const startOff = Math.max(0, isoDays(winStart.value, bar.start));
  const endOff = Math.min(winDays.value, isoDays(winStart.value, endInclusive) + 1);
  if (endOff <= startOff) return null;
  return {
    left: `${(startOff / winDays.value) * 100}%`,
    width: `${((endOff - startOff) / winDays.value) * 100}%`,
  };
}
function barLabel(bar: BoardBar): string {
  return bar.description || bar.kind || "";
}
function barTitle(bar: BoardBar): string {
  const span = bar.end && bar.end !== bar.start ? `${bar.start} / ${bar.end}` : bar.start;
  return [bar.resource, span, bar.kind && `(${bar.kind})`, bar.description]
    .filter(Boolean)
    .join(" ");
}
</script>

<template>
  <p v-if="!hasAxis" class="project-board-empty">
    {{ t("workspace.storage.board.empty") }}
  </p>
  <div v-else class="project-board" :style="{ '--n-ticks': visibleTicks.length }">
    <div class="project-board-row project-board-header">
      <div class="project-board-gutter project-board-nav">
        <button type="button" class="project-board-navbtn" :disabled="!canPrev" @click="prev">‹</button>
        <button type="button" class="project-board-navbtn" :disabled="!canNext" @click="next">›</button>
      </div>
      <div class="project-board-lane">
        <div
          v-for="(tick, i) in visibleTicks"
          :key="'t' + windowStart + '-' + i"
          class="project-board-tick"
          :style="{ gridColumn: cellColumn(i) }"
        >{{ tick.label }}</div>
      </div>
    </div>

    <div v-for="res in resources" :key="res.value" class="project-board-row">
      <div class="project-board-gutter">{{ res.label || res.value }}</div>
      <div class="project-board-lane">
        <div
          v-for="n in visibleTicks.length"
          :key="'c' + n"
          class="project-board-track-cell"
          :style="{ gridColumn: cellColumn(n - 1) }"
        ></div>
        <template v-for="bar in barsFor(res.value)" :key="'b' + bar.index">
          <div
            v-if="barStyle(bar)"
            class="project-board-bar"
            :class="{ 'is-milestone': bar.milestone }"
            :style="barStyle(bar)!"
            :title="barTitle(bar)"
            @click="emit('edit', bar.index)"
          >{{ barLabel(bar) }}</div>
        </template>
      </div>
    </div>
  </div>
</template>
