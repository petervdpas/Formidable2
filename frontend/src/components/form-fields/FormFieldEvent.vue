<script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { TextField, SelectField, DateInput, type SelectOption } from "../fields";
import { Service as TemplateSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldEvent - a single time-bar on a project board: ISO start/end,
// a kind (task/milestone/absence), and a resource. A milestone is a
// zero-span point, so its end is hidden and cleared. The kind palette is
// backend-owned (TemplateSvc.EventKinds), never a hardcoded JS list.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

type EventValue = { start: string; end: string; kind: string; resource: string };

function normalize(v: unknown): EventValue {
  if (v && typeof v === "object") {
    const o = v as Record<string, unknown>;
    return {
      start: typeof o.start === "string" ? o.start : "",
      end: typeof o.end === "string" ? o.end : "",
      kind: typeof o.kind === "string" ? o.kind : "task",
      resource: typeof o.resource === "string" ? o.resource : "",
    };
  }
  return { start: "", end: "", kind: "task", resource: "" };
}

const cur = computed<EventValue>(() => normalize(props.modelValue));

function patch(part: Partial<EventValue>) {
  emit("update:modelValue", { ...cur.value, ...part });
}

const start = computed<string>({ get: () => cur.value.start, set: (v) => patch({ start: v }) });
const end = computed<string>({ get: () => cur.value.end, set: (v) => patch({ end: v }) });
const resource = computed<string>({
  get: () => cur.value.resource,
  set: (v) => patch({ resource: v }),
});
const kind = computed<string>({
  get: () => cur.value.kind,
  // A milestone is zero-span: clear end when switching to it.
  set: (v) => patch(v === "milestone" ? { kind: v, end: "" } : { kind: v }),
});

const isMilestone = computed(() => cur.value.kind === "milestone");

// Backend-owned kind palette; its labels are i18n keys, translated here.
const kindOptions = ref<SelectOption[]>([]);
onMounted(async () => {
  try {
    const kinds = (await TemplateSvc.EventKinds()) ?? [];
    kindOptions.value = kinds.map((k) => ({ value: k.name, label: t(k.label_key) }));
  } catch {
    kindOptions.value = [];
  }
});
</script>

<template>
  <div class="event-field" :data-event-field="field.key">
    <div class="event-field-row">
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.kind") }}</label>
        <SelectField v-model="kind" :options="kindOptions" :disabled="field.readonly" />
      </div>
      <div class="event-field-stack">
        <label class="stacked-label">{{ t("field.event.resource") }}</label>
        <TextField v-model="resource" :readonly="field.readonly" />
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
  </div>
</template>
