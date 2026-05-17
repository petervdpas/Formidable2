<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import FormLoopFields from "./FormLoopFields.vue";
import FormLoopBulkToggle from "./FormLoopBulkToggle.vue";
import ConfirmDialog from "../ConfirmDialog.vue";
import { useToast } from "../../composables/useToast";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { LoopGroup } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";

const { t } = useI18n();
const toast = useToast();

// Block a drag attempt on an expanded row at the mousedown layer —
// vuedraggable hasn't started its drag tracking yet, so e.preventDefault
// on the mousedown bubbling up from the handle stops it cleanly. Toast
// gives the user immediate feedback on why nothing moved.
function onHandleMousedown(e: MouseEvent, i: number) {
  if (collapsed.value[i]) return;
  e.preventDefault();
  e.stopPropagation();
  toast.warn("workspace.storage.field.drag_collapse_first");
}

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

// modelValue may grow or shrink after mount (entry data streaming in
// async, draft restoration, parent-level mutations). Keep `collapsed`
// length-aligned: pad new indices with the group default, trim
// trailing entries when items disappear. Without this, items past
// the initial length read collapsed[i]=undefined → render expanded.
watch(
  () => props.modelValue.length,
  (next) => {
    const cur = collapsed.value.length;
    if (next === cur) return;
    if (next > cur) {
      const pad = Array(next - cur).fill(props.group.default_collapsed);
      collapsed.value = [...collapsed.value, ...pad];
    } else {
      collapsed.value = collapsed.value.slice(0, next);
    }
  },
);

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

const removeIndex = ref<number | null>(null);
const removeOpen = computed(() => removeIndex.value !== null);

function askRemove(i: number) {
  removeIndex.value = i;
}
function cancelRemove() {
  removeIndex.value = null;
}
function confirmRemove() {
  const i = removeIndex.value;
  removeIndex.value = null;
  if (i !== null) removeItem(i);
}

function toggleCollapsed(i: number) {
  const next = collapsed.value.slice();
  next[i] = !next[i];
  collapsed.value = next;
}

function setAllCollapsed(v: boolean) {
  collapsed.value = collapsed.value.map(() => v);
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
  // Link field shape `{href, text}` — prefer the human-readable text,
  // fall back to the href so the row never collapses to "[object
  // Object]". Other object shapes (rare in the summary slot) get
  // JSON-stringified for at least *some* signal in the row header.
  if (typeof v === "object") {
    const obj = v as Record<string, unknown>;
    if (typeof obj.text === "string" && obj.text.trim()) return obj.text.trim();
    if (typeof obj.href === "string" && obj.href.trim()) return obj.href.trim();
    return JSON.stringify(v);
  }
  return String(v);
}
</script>

<template>
  <div class="form-loop" :data-depth="group.depth">
    <div class="form-loop-header">
      <div class="form-loop-header-text">
        <h3 class="form-loop-title">{{ field.label || field.key }}</h3>
        <p v-if="field.description" class="form-loop-description">{{ field.description }}</p>
      </div>
      <FormLoopBulkToggle
        :collapsed="collapsed"
        @expand-all="setAllCollapsed(false)"
        @collapse-all="setAllCollapsed(true)"
      />
    </div>

    <div v-if="entries.length === 0" class="form-loop-empty muted small">
      {{ t('workspace.storage.field.loop_empty') }}
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
            <!--
              Drag handle is always visible. When the row is expanded
              we intercept mousedown on the handle, prevent the drag,
              and toast a hint to collapse first. Keeps the row
              layout stable instead of the handle disappearing
              mid-edit.
            -->
            <span
              class="dnd-handle"
              :class="{ disabled: !collapsed[i] }"
              :title="collapsed[i]
                ? t('workspace.storage.field.drag_to_reorder')
                : t('workspace.storage.field.drag_collapse_first')"
              aria-hidden="true"
              @mousedown="onHandleMousedown($event, i)"
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
              :aria-label="t('workspace.storage.field.remove_item')"
              :title="t('workspace.storage.field.remove_item')"
              @click="askRemove(i)"
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

    <div class="form-loop-actions">
      <button type="button" class="tool-btn primary" @click="addItem">
        + {{ t('workspace.storage.field.add_loop_item') }}
      </button>
    </div>

    <ConfirmDialog
      :open="removeOpen"
      :title="t('workspace.storage.field.remove_item.title')"
      :message="t('special.loop.delete.sure')"
      :confirm-label="t('common.remove')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @cancel="cancelRemove"
      @confirm="confirmRemove"
    />
  </div>
</template>

