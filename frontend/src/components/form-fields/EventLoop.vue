<script setup lang="ts">
// EventLoop - the "events" loop of a project-mode plan board. When the board has
// an axis it IS the editor: the live board (render.BuildBoardLive) shows the
// bars, clicking one edits that event in a modal, and "+ Event" adds one - the
// raw loop list is hidden. Without an axis (or a non-project template) it falls
// back to the generic FormLoop. FormLoop itself stays untouched.
import { computed, ref, watch, inject, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import FormLoop from "./FormLoop.vue";
import ProjectBoard from "../ProjectBoard.vue";
import FormFieldEvent from "./FormFieldEvent.vue";
import Modal from "../Modal.vue";
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

// The inner event field carries the kind options; FormFieldEvent needs it.
const eventField = computed<Field | undefined>(() =>
  props.innerFields.find((f) => f.type === "event"),
);

type Entry = Record<string, unknown>;
const entries = computed<Entry[]>(() =>
  props.modelValue.map((e) => (e && typeof e === "object" ? { ...(e as Entry) } : {})),
);

// Bar-click / add opens a modal editing one entry's "event" value.
const editIndex = ref<number | null>(null);
const editEvent = computed<unknown>(() =>
  editIndex.value != null ? (entries.value[editIndex.value]?.event ?? {}) : {},
);

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
function addEvent() {
  const next = [...entries.value, { event: {} }];
  emit("update:modelValue", next);
  editIndex.value = next.length - 1;
}
function writeEvent(v: unknown) {
  if (editIndex.value == null) return;
  const next = entries.value.slice();
  next[editIndex.value] = { ...next[editIndex.value], event: v };
  emit("update:modelValue", next);
}
function removeEditing() {
  if (editIndex.value == null) return;
  const next = entries.value.filter((_, i) => i !== editIndex.value);
  emit("update:modelValue", next);
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

    <Modal
      v-if="editIndex != null && eventField"
      :open="true"
      :title="t('workspace.templates.field_type.event')"
      width="640px"
      :close-on-esc="true"
      @close="editIndex = null"
    >
      <FormFieldEvent
        :field="eventField"
        :model-value="editEvent"
        @update:model-value="writeEvent"
      />
      <template #footer>
        <button type="button" class="tool-btn danger" @click="removeEditing">
          {{ t("common.remove") }}
        </button>
        <button type="button" class="tool-btn" @click="editIndex = null">
          {{ t("common.close") }}
        </button>
      </template>
    </Modal>
  </div>
</template>
