<script setup lang="ts">
import { computed } from "vue";
import SelectField from "./SelectField.vue";

// FieldSelector
// Picks one template field by key from a backend-supplied candidate
// list ({key, label} pairs). Shared by the Setup "Item Field" picker
// and the loopstart "Summary field" picker - both choose a field key
// and both lead with an optional "(none)" entry. The candidate set is
// always computed on the Go side (GetItemFields / SummaryFieldCandidates);
// this component only renders it.

const model = defineModel<string>({ default: "" });

const props = defineProps<{
  /** Candidate fields, key + display label, from the backend. */
  fields: { key: string; label: string }[];
  /** When set, prepend a clearing entry (value "") with this label.
   *  Omit to force a non-empty selection. The caller owns the string
   *  so the i18n key stays at the call site. */
  emptyLabel?: string;
  disabled?: boolean;
}>();

const options = computed(() => {
  const out: { value: string; label: string }[] = [];
  if (props.emptyLabel !== undefined) {
    out.push({ value: "", label: props.emptyLabel });
  }
  for (const f of props.fields) {
    out.push({ value: f.key, label: f.label || f.key });
  }
  return out;
});
</script>

<template>
  <SelectField v-model="model" :options="options" :disabled="disabled" />
</template>
