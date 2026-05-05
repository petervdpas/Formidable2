<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const value = computed<string>({
  get: () => (props.modelValue == null ? "" : String(props.modelValue)),
  set: (v) => emit("update:modelValue", v),
});
</script>

<template>
  <TextField type="date" v-model="value" :readonly="field.readonly" />
</template>
