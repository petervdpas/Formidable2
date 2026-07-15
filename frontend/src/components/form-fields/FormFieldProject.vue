<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldProject - the per-record value of a plan board: just the board name.
// The shared time axis (from/to dates + time-block granularity) is author-time
// config on the field options, edited in the Edit Field modal, not here.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

function normalizeName(v: unknown): string {
  if (v && typeof v === "object") {
    const o = v as Record<string, unknown>;
    return typeof o.name === "string" ? o.name : "";
  }
  return "";
}

const name = computed<string>({
  get: () => normalizeName(props.modelValue),
  set: (v) => emit("update:modelValue", { name: v }),
});
</script>

<template>
  <div class="project-field" :data-project-field="field.key">
    <div class="project-field-row">
      <div class="project-field-stack">
        <label class="stacked-label">{{ t("field.project.name") }}</label>
        <TextField v-model="name" :readonly="field.readonly" />
      </div>
    </div>
  </div>
</template>
