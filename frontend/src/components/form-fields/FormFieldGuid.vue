<script setup lang="ts">
import { computed } from "vue";
import { TextField } from "../fields";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// GUIDs are display-only - backend generates them on first save.
const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const value = computed(() => (props.modelValue == null ? "" : String(props.modelValue)));
</script>

<template>
  <TextField :model-value="value" readonly />
</template>
