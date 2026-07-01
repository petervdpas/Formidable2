<script setup lang="ts">
// Reveal "table" element. A slide table is a plain 2D string grid whose FIRST
// row is the header (matching the deck's slideTableMarkdown), so this is a
// bespoke grid editor (not the generic table field, which imposes its own
// column headers). Columns are grown/shrunk across every row; the header row is
// directly editable. The canvas shows the backend-rendered table.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import SlideRenderedPreview from "./SlideRenderedPreview.vue";
import SlideStyleControls from "./SlideStyleControls.vue";
import type { SlideBlock } from "../../../types/slide-blocks";

const props = defineProps<{ block: SlideBlock; surface: "canvas" | "inspector"; html?: string }>();
const emit = defineEmits<{ (e: "patch", p: Partial<SlideBlock>): void }>();

const { t } = useI18n();

// Normalised, padded grid with a guaranteed header row (min 2 columns).
const grid = computed<string[][]>(() => {
  const c = props.block.content;
  const rows = Array.isArray(c)
    ? (c as unknown[]).map((r) => (Array.isArray(r) ? r.map((x) => (x == null ? "" : String(x))) : []))
    : [];
  const cols = Math.max(2, ...rows.map((r) => r.length), 2);
  if (rows.length === 0) rows.push([]);
  return rows.map((r) => {
    const row = r.slice();
    while (row.length < cols) row.push("");
    return row;
  });
});
const colCount = computed(() => grid.value[0]?.length ?? 2);
const body = computed(() => grid.value.slice(1));

function emitGrid(next: string[][]) {
  emit("patch", { content: next });
}
function setCell(r: number, c: number, val: string) {
  const next = grid.value.map((row) => row.slice());
  next[r][c] = val;
  emitGrid(next);
}
function addColumn() {
  emitGrid(grid.value.map((row) => [...row, ""]));
}
function removeColumn() {
  if (colCount.value <= 2) return;
  emitGrid(grid.value.map((row) => row.slice(0, colCount.value - 1)));
}
function addRow() {
  emitGrid([...grid.value, Array.from({ length: colCount.value }, () => "")]);
}
function removeRow(bodyIdx: number) {
  emitGrid(grid.value.filter((_, i) => i !== bodyIdx + 1));
}
</script>

<template>
  <SlideRenderedPreview v-if="surface === 'canvas'" :block="block" :html="html" />
  <template v-else>
    <div class="slide-table-cols">
      <button type="button" class="btn-ghost-icon btn-sm" :title="t('workspace.storage.slide.add_column')" @click="addColumn">+</button>
      <button type="button" class="btn-ghost-icon btn-sm" :title="t('workspace.storage.slide.remove_column')" @click="removeColumn">−</button>
      <span class="slide-table-cols-count">{{ t('workspace.storage.slide.columns_count', [colCount]) }}</span>
    </div>
    <table class="slide-tablegrid">
      <thead>
        <tr>
          <th v-for="c in colCount" :key="'h' + c">
            <input
              class="slide-th-input" :value="grid[0][c - 1]"
              :placeholder="t('workspace.storage.slide.header_cell', [c])"
              @input="setCell(0, c - 1, ($event.target as HTMLInputElement).value)"
            />
          </th>
          <th class="slide-tablegrid-x" aria-hidden="true"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, ri) in body" :key="'r' + ri">
          <td v-for="c in colCount" :key="c">
            <input
              class="slide-td-input" :value="row[c - 1]"
              @input="setCell(ri + 1, c - 1, ($event.target as HTMLInputElement).value)"
            />
          </td>
          <td class="slide-tablegrid-x">
            <button type="button" class="btn-ghost-icon btn-sm" :title="t('workspace.storage.field.remove_row')" @click="removeRow(ri)">−</button>
          </td>
        </tr>
      </tbody>
    </table>
    <button type="button" class="btn-ghost-icon" :title="t('workspace.storage.field.add_row')" @click="addRow">+</button>
    <SlideStyleControls :block="block" @patch="emit('patch', $event)" />
  </template>
</template>
