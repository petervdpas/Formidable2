<script setup lang="ts">
/*
 * FormulaComputeButton - the "live" formula Compute button, rendered under a
 * target field by FormFieldRow when a live formula field writes into that field.
 *
 * The formula field itself is invisible in the form; this button is its only
 * surface. It injects the workspace's FormValues bridge, resolves the live
 * formula field whose target is `targetKey`, and triggers the backend compute
 * (which reads the SAVED record), so it is disabled while the form is
 * dirty/unsaved. Renders nothing when no live formula targets this field (so it
 * is safe to drop under every field row).
 */
import { computed, inject, ref } from "vue";
import { useI18n } from "vue-i18n";
import { FORM_VALUES_KEY } from "../../composables/formValues";

const props = defineProps<{ targetKey: string }>();

const { t } = useI18n();
const ctx = inject(FORM_VALUES_KEY, null);

const formulaFieldKey = computed<string | null>(
  () => ctx?.liveFormulaTargets.value[props.targetKey] ?? null,
);
const canCompute = computed<boolean>(
  () => !!ctx && ctx.saved.value && !ctx.dirty.value,
);

const computing = ref(false);
async function run() {
  if (!ctx || !formulaFieldKey.value || computing.value) return;
  computing.value = true;
  try {
    await ctx.compute(formulaFieldKey.value);
  } finally {
    computing.value = false;
  }
}
</script>

<template>
  <button
    v-if="formulaFieldKey"
    type="button"
    class="tool-btn small formula-compute-btn"
    :disabled="!canCompute || computing"
    :title="canCompute ? t('formula.field.compute') : t('formula.field.compute_dirty')"
    @click="run"
  >
    {{ t("formula.field.compute") }}
  </button>
</template>
