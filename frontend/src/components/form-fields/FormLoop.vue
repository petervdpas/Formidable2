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
      handle=".dnd-handle"
      :animation="150"
      ghost-class="dnd-ghost"
      chosen-class="dnd-chosen"
      drag-class="dnd-drag"
      :item-key="(_e: Record<string, unknown>, i: number) => i"
      @change="onDragChange"
    >
      <template #item="{ index: i, element: entry }">
        <div :class="['form-loop-item', { collapsed: collapsed[i] }]">
          <div class="form-loop-item-header">
            <span
              class="dnd-handle"
              :title="'Drag to reorder'"
              aria-hidden="true"
            >⠿</span>

            <button
              type="button"
              class="btn-ghost-icon btn-sm"
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
              class="btn-ghost-icon btn-md"
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

    <button type="button" class="btn-ghost-block" @click="addItem">+ Add</button>
  </div>
</template>

