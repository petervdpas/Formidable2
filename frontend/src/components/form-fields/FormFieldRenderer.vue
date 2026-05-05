<script setup lang="ts">
import { computed, type Component } from "vue";
import FormFieldText from "./FormFieldText.vue";
import FormFieldBoolean from "./FormFieldBoolean.vue";
import FormFieldDropdown from "./FormFieldDropdown.vue";
import FormFieldTextarea from "./FormFieldTextarea.vue";
import FormFieldNumber from "./FormFieldNumber.vue";
import FormFieldRange from "./FormFieldRange.vue";
import FormFieldDate from "./FormFieldDate.vue";
import FormFieldMultioption from "./FormFieldMultioption.vue";
import FormFieldRadio from "./FormFieldRadio.vue";
import FormFieldList from "./FormFieldList.vue";
import FormFieldTable from "./FormFieldTable.vue";
import FormFieldGuid from "./FormFieldGuid.vue";
import FormFieldTags from "./FormFieldTags.vue";
import FormFieldUnknown from "./FormFieldUnknown.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// Per-type dispatch table. Adding a new renderer is one line here.
// "loopstart"/"loopstop" never reach this component — the workspace
// handles them at the iteration level (loop containers).
const DISPATCH: Record<string, Component> = {
  text: FormFieldText,
  boolean: FormFieldBoolean,
  dropdown: FormFieldDropdown,
  textarea: FormFieldTextarea,
  number: FormFieldNumber,
  range: FormFieldRange,
  date: FormFieldDate,
  multioption: FormFieldMultioption,
  radio: FormFieldRadio,
  list: FormFieldList,
  table: FormFieldTable,
  guid: FormFieldGuid,
  tags: FormFieldTags,
};

const component = computed<Component>(
  () => DISPATCH[props.field.type] ?? FormFieldUnknown,
);
</script>

<template>
  <component
    :is="component"
    :field="field"
    :model-value="modelValue"
    @update:model-value="(v: unknown) => $emit('update:modelValue', v)"
  />
</template>
