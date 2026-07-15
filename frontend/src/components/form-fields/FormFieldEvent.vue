<script setup lang="ts">
import { computed, ref, onMounted, inject, watch, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { TextField, SelectField, DateInput, type SelectOption } from "../fields";
import { Service as TemplateSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldEvent - a placement on the project board's two axes: X (time) via
// start/end, Y (resource) via a picker sourced from the project's resources.
// Kind (task/milestone/absence) is backend-owned; description is a free-text
// note. A milestone is a zero-span point, so its end is hidden.

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
  description: string;
};

function normalize(v: unknown): EventValue {
  const o = (v && typeof v === "object" ? v : {}) as Record<string, unknown>;
  const str = (x: unknown, d = "") => (typeof x === "string" ? x : d);
  return {
    start: str(o.start),
    end: str(o.end),
    kind: str(o.kind, "task"),
    resource: str(o.resource),
    description: str(o.description),
  };
}

const cur = computed<EventValue>(() => normalize(props.modelValue));

function patch(part: Partial<EventValue>) {
  emit("update:modelValue", { ...cur.value, ...part });
}

const start = computed<string>({ get: () => cur.value.start, set: (v) => patch({ start: v }) });
const end = computed<string>({ get: () => cur.value.end, set: (v) => patch({ end: v }) });
const resource = computed<string>({ get: () => cur.value.resource, set: (v) => patch({ resource: v }) });
const description = computed<string>({
  get: () => cur.value.description,
  set: (v) => patch({ description: v }),
});
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
async function loadResources() {
  const tpl = templateFilename.value;
  if (!tpl) {
    resourceOptions.value = [];
    return;
  }
  try {
    const rs = (await TemplateSvc.ProjectResources(tpl)) ?? [];
    resourceOptions.value = rs.map((r) => ({ value: r.value, label: r.label || r.value }));
  } catch {
    resourceOptions.value = [];
  }
}
onMounted(loadResources);
watch(() => templateFilename.value, loadResources);
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
        <DateInput v-model="start" :readonly="field.readonly" />
      </div>
      <div v-if="!isMilestone" class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.end") }}</label>
        <DateInput v-model="end" :readonly="field.readonly" />
      </div>
    </div>
    <div class="event-field-row">
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.description") }}</label>
        <TextField v-model="description" :readonly="field.readonly" />
      </div>
    </div>
  </div>
</template>
