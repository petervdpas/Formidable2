<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { TextField, SelectField } from "../fields";
import { timeBlocks } from "../../types/option-presets";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// FormFieldProject - the per-record value of a plan board: the board name and an
// optional time-block override. The shared axis window (from/to) and the default
// granularity are author-time config on the field options (Edit Field modal);
// here the record may re-tick the axis to a different granularity. Empty override
// tracks the template default.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

const blockOptions = timeBlocks();

// Patch one property, preserving the rest of the value object (name /
// resourceOrder / timeBlock never clobber each other).
function baseValue(): Record<string, unknown> {
  const v = props.modelValue;
  return v && typeof v === "object" ? { ...(v as Record<string, unknown>) } : {};
}
function patch(part: Record<string, unknown>) {
  emit("update:modelValue", { ...baseValue(), ...part });
}

const name = computed<string>({
  get: () => {
    const v = props.modelValue;
    return v && typeof v === "object" && typeof (v as Record<string, unknown>).name === "string"
      ? ((v as Record<string, unknown>).name as string)
      : "";
  },
  set: (v) => patch({ name: v }),
});

// The template's authored default (project field option row value="timeblock"),
// falling back to "week" like the backend's ProjectTimeBlock.
function defaultTimeBlock(): string {
  const opts = props.field.options;
  if (Array.isArray(opts)) {
    for (const o of opts) {
      if (o && typeof o === "object" && (o as Record<string, unknown>).value === "timeblock") {
        const lab = (o as Record<string, unknown>).label;
        if (typeof lab === "string" && lab) return lab;
      }
    }
  }
  return "week";
}

const timeBlock = computed<string>({
  get: () => {
    const v = props.modelValue;
    const tb = v && typeof v === "object" ? (v as Record<string, unknown>).timeBlock : null;
    return typeof tb === "string" && tb ? tb : defaultTimeBlock();
  },
  set: (v) => patch({ timeBlock: v }),
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
    <div class="project-field-row">
      <div class="project-field-stack project-field-stack--narrow">
        <label class="stacked-label">{{ t("field.project.timeblock") }}</label>
        <SelectField v-model="timeBlock" :options="blockOptions" :disabled="field.readonly" />
        <span class="project-field-hint">{{ t("field.project.timeblock_hint") }}</span>
      </div>
    </div>
  </div>
</template>
