<script setup lang="ts">
// EventLoop - the "events" loop of a project-mode plan board. When the board has
// an axis it IS the editor: the live board (render.BuildBoardLive) shows the
// bars, clicking one edits that event in a modal, and "+ Event" adds one - the
// raw loop list is hidden. Without an axis (or a non-project template) it falls
// back to the generic FormLoop. FormLoop itself stays untouched.
import { computed, ref, watch, inject, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import FormLoop from "./FormLoop.vue";
import FormLoopFields from "./FormLoopFields.vue";
import ProjectBoard from "../ProjectBoard.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { LoopGroup } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { Board } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";

const props = defineProps<{
  field: Field;
  group: LoopGroup;
  innerFields: Field[];
  innerStartOffset: number;
  loopGroups: LoopGroup[];
  modelValue: unknown[];
  // The form's live record values, so the board can read/write this record's
  // resource (Y-axis) order on the project value - drag-to-sort is a normal
  // record edit that marks the form dirty and saves with it.
  values: Record<string, unknown>;
}>();
const emit = defineEmits<{ (e: "update:modelValue", v: unknown[]): void }>();

const { t } = useI18n();

const templateFilename = inject<Ref<string>>("templateFilename", ref(""));
const liveBoard = ref<Board | null>(null);

// This record's resource order lives on the project value ({name, resourceOrder}).
const resourceOrder = computed<string[]>(() => {
  const p = props.values["project"];
  const arr = p && typeof p === "object" ? (p as { resourceOrder?: unknown }).resourceOrder : null;
  return Array.isArray(arr) ? (arr.filter((x) => typeof x === "string") as string[]) : [];
});

async function refresh() {
  if (!templateFilename.value) {
    liveBoard.value = null;
    return;
  }
  try {
    liveBoard.value = await RenderSvc.BuildBoardLive(
      templateFilename.value,
      "",
      props.modelValue,
      resourceOrder.value,
    );
  } catch {
    liveBoard.value = null;
  }
}
watch([() => props.modelValue, () => templateFilename.value, resourceOrder], refresh, {
  immediate: true,
  deep: true,
});
const showBoard = computed(() => (liveBoard.value?.ticks?.length ?? 0) > 0);

type Entry = Record<string, unknown>;
const entries = computed<Entry[]>(() =>
  props.modelValue.map((e) => (e && typeof e === "object" ? { ...(e as Entry) } : {})),
);

// Clicking a bar (or + Event) opens the editor for one loop iteration. The
// editor is a real loop item: it edits EVERY inner field the author put in the
// events loop (the event bar plus any siblings like a description), not just the
// baked-in event value.
const editIndex = ref<number | null>(null);

function openEdit(index: number) {
  editIndex.value = index;
}

// Drag-sorting the Y-axis writes the new order onto this record's project value.
// That's a normal record edit: it marks the form dirty and the board rebuilds
// (the resourceOrder watch → refresh); the user's Save persists it.
function reorderResources(order: string[]) {
  const p = props.values["project"];
  const base = p && typeof p === "object" ? { ...(p as Record<string, unknown>) } : {};
  props.values["project"] = { ...base, resourceOrder: order };
}

// setEntry re-emits the whole array up, keeping the value flow one-way (mirrors
// FormLoop).
function setEntry(i: number, key: string, val: unknown) {
  const next = entries.value.slice();
  next[i] = { ...next[i], [key]: val };
  emit("update:modelValue", next);
}

// The events looper folds author-added fields INTO the event: an iteration is
// stored as { event: { resource, kind, start, end, ...authorFields } }, not the
// event plus sibling keys. So the editor's proxy routes every field except
// "event" itself through entry.event: FormFieldEvent binds to the whole event
// object, a "description" field binds to event.description, etc. Reading and
// writing both go through event, so nothing lands as a stray sibling key.
function eventOf(i: number): Entry {
  const ev = entries.value[i]?.event;
  return ev && typeof ev === "object" ? (ev as Entry) : {};
}
function entryProxy(i: number): Entry {
  return new Proxy(
    {},
    {
      get: (_, key) =>
        key === "event" ? entries.value[i]?.event : eventOf(i)[key as string],
      set: (_, key, val) => {
        if (key === "event") setEntry(i, "event", val);
        else setEntry(i, "event", { ...eventOf(i), [key as string]: val });
        return true;
      },
    },
  );
}

function addEvent() {
  const next = [...entries.value, {}];
  emit("update:modelValue", next);
  editIndex.value = next.length - 1;
}
function removeEditing() {
  if (editIndex.value == null) return;
  const next = entries.value.filter((_, i) => i !== editIndex.value);
  emit("update:modelValue", next);
  editIndex.value = null;
}
// Abandoned-blank iterations are NOT pruned here: that "loopers never persist
// empty entries" invariant lives in the backend (storage.Sanitize, on save), so
// there's one source of truth. A blank event has no start, so it draws no bar
// anyway; Save removes it from disk.
function closeEditor() {
  editIndex.value = null;
}
</script>

<template>
  <div class="event-loop">
    <template v-if="showBoard">
      <div class="event-loop-header">
        <div class="form-loop-header-text">
          <h3 class="form-loop-title">{{ field.label || field.key }}</h3>
          <p v-if="field.description" class="form-loop-description">{{ field.description }}</p>
        </div>
      </div>
      <ProjectBoard :board="liveBoard" @edit="openEdit" @reorder="reorderResources" />

      <button type="button" class="tool-btn primary event-loop-add" @click="addEvent">
        + {{ t("workspace.templates.field_type.event") }}
      </button>

      <!-- The event editor lives inline, not in a modal: on a board you pick a
           bar and tweak it in place, watching the timeline react. It opens
           below the add button so that button stays anchored to the chart.
           It edits a whole loop iteration via FormLoopFields, so every inner
           field the author added to the events loop shows up, not just the
           event bar. Only one editor is open at a time (editIndex). -->
      <div v-if="editIndex != null" class="event-loop-editor">
        <div class="event-loop-editor-head">
          <h4 class="event-loop-editor-title">
            {{ t("workspace.templates.field_type.event") }} #{{ editIndex + 1 }}
          </h4>
          <button
            type="button"
            class="event-loop-editor-close"
            :aria-label="t('common.close')"
            @click="closeEditor"
          >
            &times;
          </button>
        </div>
        <FormLoopFields
          :fields="innerFields"
          :start-offset="innerStartOffset"
          :values="entryProxy(editIndex)"
          :loop-groups="loopGroups"
        />
        <div class="event-loop-editor-actions">
          <button type="button" class="tool-btn danger" @click="removeEditing">
            {{ t("common.remove") }}
          </button>
          <button type="button" class="tool-btn" @click="closeEditor">
            {{ t("common.close") }}
          </button>
        </div>
      </div>
    </template>

    <FormLoop
      v-else
      :field="field"
      :group="group"
      :inner-fields="innerFields"
      :inner-start-offset="innerStartOffset"
      :loop-groups="loopGroups"
      :model-value="modelValue"
      @update:model-value="(v: unknown[]) => emit('update:modelValue', v)"
    />
  </div>
</template>
