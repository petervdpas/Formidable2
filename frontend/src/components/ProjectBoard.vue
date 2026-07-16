<script setup lang="ts">
// Plan-board grid (presentational): draws a structured board (render.Board) as a
// time (X) by resource (Y) grid. Backend does the date math; this lays it out.
// The X axis is windowed (a few columns at a time, move left/right); the Y axis
// (resource gutter) stays visible. Clicking a bar emits its source entry index.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import type { Board, BoardBar } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { ResourceDescriptor } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = withDefaults(
  defineProps<{
    board: Board | null;
    /** How many time columns to show at once (default 4). */
    windowSize?: number;
  }>(),
  { windowSize: 4 },
);
const emit = defineEmits<{
  (e: "edit", index: number): void;
  (e: "reorder", order: string[]): void;
}>();

function onReorder(next: ResourceDescriptor[]) {
  emit("reorder", next.map((r) => r.value));
}

const { t } = useI18n();

const allTicks = computed(() => props.board?.ticks ?? []);
const resources = computed(() => props.board?.resources ?? []);
const hasAxis = computed(() => allTicks.value.length > 0 && resources.value.length > 0);

// X-axis window: show `size` columns (zoom) starting at windowStart (pan). size
// is capped at the total tick count, so a short project just shows all of it.
const size = ref(props.windowSize);
const windowStart = ref(0);
// Zoom-out cap depends on granularity: 8 columns for days, 6 for weeks / 2- and
// 3-week sprints / months (and never more than the project has).
const maxUnits = computed(() => (props.board?.time_block === "day" ? 8 : 6));
const zoomMax = computed(() => Math.min(allTicks.value.length, maxUnits.value));
const visibleCount = computed(() => Math.max(1, Math.min(size.value, zoomMax.value)));
const maxStart = computed(() => Math.max(0, allTicks.value.length - visibleCount.value));
watch(maxStart, (m) => {
  if (windowStart.value > m) windowStart.value = m;
});
const visibleTicks = computed(() =>
  allTicks.value.slice(windowStart.value, windowStart.value + visibleCount.value),
);
const canPrev = computed(() => windowStart.value > 0);
const canNext = computed(() => windowStart.value < maxStart.value);
function prev() {
  if (canPrev.value) windowStart.value -= 1;
}
function next() {
  if (canNext.value) windowStart.value += 1;
}

// Zoom: "-" shows fewer time units, "+" shows more (up to the whole project).
const canZoomIn = computed(() => visibleCount.value > 1);
const canZoomOut = computed(() => visibleCount.value < zoomMax.value);
function zoomIn() {
  if (canZoomIn.value) size.value = visibleCount.value - 1;
}
function zoomOut() {
  if (canZoomOut.value) size.value = visibleCount.value + 1;
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
function barStyle(bar: BoardBar): Record<string, string> | null {
  if (!winStart.value) return null;
  const endInclusive = bar.end && bar.end >= bar.start ? bar.end : bar.start;
  const startOff = Math.max(0, isoDays(winStart.value, bar.start));
  const endOff = Math.min(winDays.value, isoDays(winStart.value, endInclusive) + 1);
  if (endOff <= startOff) return null;
  const style: Record<string, string> = {
    left: `${(startOff / winDays.value) * 100}%`,
    width: `${((endOff - startOff) / winDays.value) * 100}%`,
  };
  // A milestone keeps its diamond colour rule; a bar takes its kind colour.
  if (bar.color && !bar.milestone) style.background = bar.color;
  return style;
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
        <button type="button" class="project-board-navbtn" :disabled="!canZoomIn" title="Zoom in" @click="zoomIn">−</button>
        <button type="button" class="project-board-navbtn" :disabled="!canZoomOut" title="Zoom out" @click="zoomOut">+</button>
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

    <draggable
      :model-value="resources"
      tag="div"
      item-key="value"
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      @update:model-value="onReorder"
    >
      <template #item="{ element: res }">
        <div class="project-board-row">
          <div class="project-board-gutter">
            <span class="dnd-handle" aria-hidden="true">⠿</span>
            <span class="project-board-res">{{ res.label || res.value }}</span>
          </div>
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
      </template>
    </draggable>
  </div>
</template>
