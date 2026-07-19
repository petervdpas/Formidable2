<script setup lang="ts">
import { computed, ref, onMounted, inject, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, DateInput, type SelectOption } from "../fields";
import { Service as TemplateSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldEvent - a placement on the project board's two axes: X (time) via
// start/end, Y (resource) via a picker sourced from the project's resources.
// Kind is author-defined on the event field's options. A note about the bar is
// NOT part of the event: add a sibling field to the events loop for that. A
// milestone is a zero-span point, so its end is hidden.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

type EventValue = {
  start: string;
  end: string;
  kind: string;
  resource: string;
};

function normalize(v: unknown): EventValue {
  const o = (v && typeof v === "object" ? v : {}) as Record<string, unknown>;
  const str = (x: unknown, d = "") => (typeof x === "string" ? x : d);
  return {
    start: str(o.start),
    end: str(o.end),
    kind: str(o.kind, "task"),
    resource: str(o.resource),
  };
}

const cur = computed<EventValue>(() => normalize(props.modelValue));

// Patch over the RAW value, not the normalized axes: the events looper folds
// author-added fields (e.g. description) into the event object, so editing an
// axis must preserve every other key rather than drop it back to {start,end,
// kind,resource}.
function patch(part: Partial<EventValue>) {
  const base =
    props.modelValue && typeof props.modelValue === "object"
      ? (props.modelValue as Record<string, unknown>)
      : {};
  emit("update:modelValue", { ...base, ...part });
}

const start = computed<string>({ get: () => cur.value.start, set: (v) => patch({ start: v }) });
const end = computed<string>({ get: () => cur.value.end, set: (v) => patch({ end: v }) });
const resource = computed<string>({ get: () => cur.value.resource, set: (v) => patch({ resource: v }) });
const kind = computed<string>({
  get: () => cur.value.kind,
  // A milestone is zero-span: clear end when switching to it.
  set: (v) => patch(v === "milestone" ? { kind: v, end: "" } : { kind: v }),
});

const isMilestone = computed(() => cur.value.kind === "milestone");

// Kind vocabulary is author-defined on the event field's options. No fallback:
// the template can't be saved without at least one kind (backend enforces it).
const kindOptions = computed<SelectOption[]>(() => {
  const opts = (props.field.options ?? []) as Array<Record<string, unknown>>;
  return opts
    .map((o) => ({
      value: typeof o.value === "string" ? o.value : "",
      label: typeof o.label === "string" && o.label ? o.label : String(o.value ?? ""),
    }))
    .filter((o) => o.value);
});

// Resource palette (the Y axis) comes from the project field's options on the
// same template. Backend drives it: TemplateSvc.ProjectResources, never a local
// list.
const templateFilename = inject<Ref<string>>("templateFilename", ref(""));
const resourceOptions = ref<SelectOption[]>([]);
// The project's authored axis window; events can't be dated outside it, so the
// Start/End pickers clamp to [from, to].
const rangeFrom = ref("");
const rangeTo = ref("");
async function loadProject() {
  const tpl = templateFilename.value;
  if (!tpl) {
    resourceOptions.value = [];
    rangeFrom.value = "";
    rangeTo.value = "";
    return;
  }
  try {
    const rs = (await TemplateSvc.ProjectResources(tpl)) ?? [];
    resourceOptions.value = rs.map((r) => ({ value: r.value, label: r.label || r.value }));
  } catch {
    resourceOptions.value = [];
  }
  try {
    const range = (await TemplateSvc.ProjectDateRange(tpl)) ?? [];
    rangeFrom.value = range[0] ?? "";
    rangeTo.value = range[1] ?? "";
  } catch {
    rangeFrom.value = "";
    rangeTo.value = "";
  }
}
onMounted(loadProject);
watch(() => templateFilename.value, loadProject);
</script>

<template>
  <div class="event-field" :data-event-field="field.key">
    <div class="event-field-row">
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.resource") }}</label>
        <SelectField v-model="resource" :options="resourceOptions" :disabled="field.readonly" />
      </div>
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.kind") }}</label>
        <SelectField v-model="kind" :options="kindOptions" :disabled="field.readonly" />
      </div>
    </div>
    <div class="event-field-row">
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.start") }}</label>
        <DateInput v-model="start" :readonly="field.readonly" :min="rangeFrom" :max="rangeTo" />
      </div>
      <div v-if="!isMilestone" class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.end") }}</label>
        <DateInput v-model="end" :readonly="field.readonly" :min="start || rangeFrom" :max="rangeTo" />
      </div>
    </div>
  </div>
</template>
