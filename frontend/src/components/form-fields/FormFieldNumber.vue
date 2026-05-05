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
  set: (raw) => {
    const n = Number(raw);
    emit("update:modelValue", Number.isFinite(n) ? n : 0);
  },
});
</script>

<template>
  <TextField type="number" v-model="value" :readonly="field.readonly" />
</template>
