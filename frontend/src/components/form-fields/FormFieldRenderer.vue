<script setup lang="ts">
import { computed, type Component } from "vue";
import FormFieldText from "./FormFieldText.vue";
import FormFieldFilePath from "./FormFieldFilePath.vue";
import FormFieldFolderPath from "./FormFieldFolderPath.vue";
import FormFieldBoolean from "./FormFieldBoolean.vue";
import FormFieldDropdown from "./FormFieldDropdown.vue";
import FormFieldTextarea from "./FormFieldTextarea.vue";
import FormFieldMermaid from "./FormFieldMermaid.vue";
import FormFieldSlide from "./FormFieldSlide.vue";
import FormFieldNumber from "./FormFieldNumber.vue";
import FormFieldSequence from "./FormFieldSequence.vue";
import FormFieldRange from "./FormFieldRange.vue";
import FormFieldDate from "./FormFieldDate.vue";
import FormFieldMultioption from "./FormFieldMultioption.vue";
import FormFieldRadio from "./FormFieldRadio.vue";
import FormFieldList from "./FormFieldList.vue";
import FormFieldTable from "./FormFieldTable.vue";
import FormFieldGuid from "./FormFieldGuid.vue";
import FormFieldTags from "./FormFieldTags.vue";
import FormFieldImage from "./FormFieldImage.vue";
import FormFieldLink from "./FormFieldLink.vue";
import FormFieldAPI from "./FormFieldAPI.vue";
import FormFieldFacet from "./FormFieldFacet.vue";
import FormFieldUnknown from "./FormFieldUnknown.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

// Per-type dispatch table. Adding a new renderer is one line here.
// "loopstart"/"loopstop" never reach this component - the workspace
// handles them at the iteration level (loop containers).
const DISPATCH: Record<string, Component> = {
  text: FormFieldText,
  "file-path": FormFieldFilePath,
  "folder-path": FormFieldFolderPath,
  boolean: FormFieldBoolean,
  dropdown: FormFieldDropdown,
  slideset: FormFieldDropdown,
  textarea: FormFieldTextarea,
  mermaid: FormFieldMermaid,
  slide: FormFieldSlide,
  number: FormFieldNumber,
  sequence: FormFieldSequence,
  range: FormFieldRange,
  date: FormFieldDate,
  multioption: FormFieldMultioption,
  radio: FormFieldRadio,
  list: FormFieldList,
  table: FormFieldTable,
  guid: FormFieldGuid,
  tags: FormFieldTags,
  image: FormFieldImage,
  link: FormFieldLink,
  api: FormFieldAPI,
  facet: FormFieldFacet,
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
