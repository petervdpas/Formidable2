<script setup lang="ts">
import { computed, ref } from "vue";
import draggable from "vuedraggable";
import FormLoopFields from "./FormLoopFields.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { LoopGroup } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";

// FormLoop renders one loopstart/loopstop pair as a list of items.
// Each item's value is a Record<string, unknown> with the inner
// fields' keys. Add/Remove emit a fresh array up to the parent.
//
// The summary line shows when an item is collapsed; we read the
// `summary_field_key` from the LoopGroup and pull the value from the
// item's record.

const props = defineProps<{
  field: Field;                                  // the loopstart field
  group: LoopGroup;                              // precomputed pairing
  innerFields: Field[];                          // fields between start+1 and stop
  innerStartOffset: number;                      // absolute offset of innerFields[0]
  loopGroups: LoopGroup[];                       // all groups (for nested lookup)
  modelValue: unknown[];                         // array of entries
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown[]): void }>();

// Each item's collapsed state is local UI — independent per item,
// initialized from the group's default and never persisted.
const collapsed = ref<boolean[]>(props.modelValue.map(() => props.group.default_collapsed));

// DnD scope — unique per loop instance. Prevents Sortable from
// accepting items dragged from a sibling or nested loop. The
// loopstart's start_index is unique within a template, and
// loop-{key}-{start_index} survives nesting cleanly.
const dndScope = computed(() => `loop-${props.group.key}-${props.group.start_index}`);

// Coerce any entry to a plain Record for FormLoopFields to bind to.
function asEntry(v: unknown): Record<string, unknown> {
  if (v && typeof v === "object" && !Array.isArray(v)) {
    return v as Record<string, unknown>;
  }
  return {};
}

const entries = computed<Record<string, unknown>[]>(() =>
  props.modelValue.map(asEntry),
);

function emitEntries(next: Record<string, unknown>[]) {
  emit("update:modelValue", next);
}

function addItem() {
  const next = [...entries.value, {}];
  collapsed.value = [...collapsed.value, props.group.default_collapsed];
  emitEntries(next);
}

function removeItem(i: number) {
  const next = entries.value.slice();
  next.splice(i, 1);
  collapsed.value = collapsed.value.slice();
  collapsed.value.splice(i, 1);
  emitEntries(next);
}

function toggleCollapsed(i: number) {
  const next = collapsed.value.slice();
  next[i] = !next[i];
  collapsed.value = next;
}

// When the inner walker mutates an entry (assigns to its `values`),
// `entries.value[i]` is the same object reference the parent array
// holds, so reactivity flows. But because props.modelValue is read-
// only by convention, we also re-emit on every set so the upstream
// draft tracks it. The cheapest route is to reassign the inner
// record into the array slot.
function setEntry(i: number, key: string, val: unknown) {
  const next = entries.value.slice();
  next[i] = { ...next[i], [key]: val };
  emitEntries(next);
}

// Bridge: FormLoopFields binds to a `values` prop. We hand it a
// proxy object that intercepts writes and routes them through
// setEntry, so the value flow stays one-way (up).
function entryProxy(i: number): Record<string, unknown> {
  const target = entries.value[i] ?? {};
  return new Proxy(target, {
    get(_, key) {
      return entries.value[i]?.[key as string];
    },
    set(_, key, val) {
      setEntry(i, key as string, val);
      return true;
    },
  });
}

// Writable model for vuedraggable's v-model — get returns the
// computed entries; set re-emits up the tree. The same one-way
// flow we used for setEntry, just driven by the array reorder.
const draggableEntries = computed<Record<string, unknown>[]>({
  get: () => entries.value,
  set: (next) => emitEntries(next),
});

// vuedraggable @change fires after a successful drag. We use
// `moved` to keep collapsed[] aligned with the new entry order so
// per-item collapse state travels with the item.
function onDragChange(evt: { moved?: { oldIndex: number; newIndex: number } }) {
  if (!evt.moved) return;
  const { oldIndex, newIndex } = evt.moved;
  const next = collapsed.value.slice();
  const [item] = next.splice(oldIndex, 1);
  next.splice(newIndex, 0, item);
  collapsed.value = next;
}

function summaryFor(entry: Record<string, unknown>): string {
  const key = props.group.summary_field_key;
  if (!key) return "";
  const v = entry[key];
  if (v == null) return "";
  if (typeof v === "string") {
    return v.split("\n")[0].trim() || "";
  }
  return String(v);
}
</script>

<template>
  <div class="form-loop" :data-depth="group.depth">
    <div class="form-loop-header">
      <h3 class="form-loop-title">{{ field.label || field.key }}</h3>
      <p v-if="field.description" class="form-loop-description">{{ field.description }}</p>
    </div>

    <div v-if="entries.length === 0" class="form-loop-empty muted small">
      (No entries — click + to add one)
    </div>

    <draggable
      v-else
      v-model="draggableEntries"
      tag="div"
      class="form-loop-list"
      :data-dnd-scope="dndScope"
      :group="dndScope"
      handle=".form-loop-handle"
      :animation="150"
      ghost-class="form-loop-item-ghost"
      chosen-class="form-loop-item-chosen"
      drag-class="form-loop-item-drag"
      :item-key="(_e: Record<string, unknown>, i: number) => i"
      @change="onDragChange"
    >
      <template #item="{ index: i, element: entry }">
        <div :class="['form-loop-item', { collapsed: collapsed[i] }]">
          <div class="form-loop-item-header">
            <span
              class="form-loop-handle"
              :title="'Drag to reorder'"
              aria-hidden="true"
            >⠿</span>

            <button
              type="button"
              class="form-loop-toggle"
              :aria-expanded="!collapsed[i]"
              @click="toggleCollapsed(i)"
            >{{ collapsed[i] ? '▶' : '▼' }}</button>

            <span class="form-loop-item-index">#{{ i + 1 }}</span>

            <span v-if="collapsed[i]" class="form-loop-item-summary">
              {{ summaryFor(entry) || '(empty)' }}
            </span>

            <span class="form-loop-item-spacer"></span>

            <button
              type="button"
              class="form-loop-remove"
              :aria-label="'Remove item ' + (i + 1)"
              @click="removeItem(i)"
            >−</button>
          </div>

          <div v-if="!collapsed[i]" class="form-loop-item-body">
            <FormLoopFields
              :fields="innerFields"
              :start-offset="innerStartOffset"
              :values="entryProxy(i)"
              :loop-groups="loopGroups"
            />
          </div>
        </div>
      </template>
    </draggable>

    <button type="button" class="form-loop-add" @click="addItem">+ Add</button>
  </div>
</template>

<style scoped>
.form-loop {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    margin: var(--space-3) 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-surface);
}
.form-loop[data-depth="2"] {
    /* Slight indent + tinted background so nested loops read as
       "inside" their parent rather than as siblings. */
    background: var(--color-surface-2);
    margin-left: var(--space-3);
}
.form-loop-title {
    margin: 0;
    font-size: var(--font-size-md);
    font-weight: 600;
}
.form-loop-description {
    margin: 4px 0 0;
    font-size: var(--font-size-sm);
    color: var(--color-muted, #6b7280);
}
.form-loop-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
}
.form-loop-item {
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg);
}
.form-loop-item-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 6px 8px;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-surface);
    border-radius: var(--radius-md) var(--radius-md) 0 0;
}
.form-loop-item.collapsed .form-loop-item-header {
    border-bottom: 0;
    border-radius: var(--radius-md);
}
.form-loop-item-index {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--color-muted, #6b7280);
}
.form-loop-item-summary {
    font-size: var(--font-size-sm);
    color: var(--color-text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 60%;
}
.form-loop-handle {
    cursor: grab;
    user-select: none;
    font-size: 16px;
    line-height: 1;
    opacity: 0.7;
    padding: 0 2px;
}
.form-loop-handle:active { cursor: grabbing; }
.form-loop-item-spacer { flex: 1 1 auto; }

/* Sortable.js visual states (vuedraggable forwards class names). */
.form-loop-item-ghost {
    opacity: 0.35;
    filter: saturate(0.4);
}
.form-loop-item-chosen {
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
}
.form-loop-item-drag {
    cursor: grabbing;
}
.form-loop-toggle,
.form-loop-remove,
.form-loop-add {
    appearance: none;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    color: var(--color-text);
    border-radius: var(--radius-md);
    cursor: pointer;
    line-height: 1;
    font-weight: 600;
}
.form-loop-toggle {
    width: 24px;
    height: 24px;
    font-size: 11px;
}
.form-loop-remove {
    width: 28px;
    height: 28px;
    font-size: 14px;
}
.form-loop-toggle:hover,
.form-loop-remove:hover,
.form-loop-add:hover { background: var(--color-surface-2); }
.form-loop-add {
    width: 100%;
    padding: 6px;
    font-size: 14px;
}
.form-loop-item-body {
    padding: 0 var(--space-3) var(--space-2);
}
.form-loop-empty {
    padding: 8px 0;
}
</style>
