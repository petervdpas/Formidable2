<script setup lang="ts">
/*
 * FormFieldFacet - first VIRTUAL field renderer.
 *
 * A virtual field participates in template layout but does NOT carry a
 * value in storage Form.data. The facet variant reads + writes the
 * record's meta.facets[<facet_key>] state, the same slot the
 * StorageMetaBlock corner picker drives. Both setters stay in sync
 * because they write the same key on the same draft.
 *
 * Wiring: the parent workspace provides `facetContext` (facets list +
 * current state map + onChange emit). This component injects it and
 * looks up its bound facet by `field.facet_key`. When inject is
 * absent (e.g. a plugin-form rendering FormFieldRow in isolation) the
 * field stays inert with a small "not available" hint.
 *
 * Presentation:
 *   field.format = "dropdown"  → SelectField with a clear/none entry
 *   field.format = "radio" / "" → vertical radio list with color swatches
 */
import { computed, inject } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, type SelectOption } from "../fields";
import FacetIcon from "../FacetIcon.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { FacetState } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import { FACET_CONTEXT_KEY, type FacetContext } from "../../composables/facetContext";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const { t } = useI18n();

const ctx = inject<FacetContext | null>(FACET_CONTEXT_KEY, null);

const facetKey = computed(() => props.field.facet_key ?? "");

const facet = computed(() => {
  if (!ctx || !facetKey.value) return null;
  return ctx.facets.value.find((f) => f.key === facetKey.value) ?? null;
});

const state = computed<FacetState>(() => {
  if (!ctx) return new FacetState({ set: false, selected: "" });
  const entry = ctx.state.value[facetKey.value];
  return entry
    ? new FacetState({ set: entry.set, selected: entry.selected ?? "" })
    : new FacetState({ set: false, selected: "" });
});

const presentation = computed<"radio" | "dropdown">(() => {
  return props.field.format === "dropdown" ? "dropdown" : "radio";
});

const selectOptions = computed<SelectOption[]>(() => {
  if (!facet.value) return [];
  return facet.value.options.map((o) => ({ value: o.label, label: o.label }));
});

const dropdownValue = computed<string>({
  get: () => state.value.selected || "",
  set: (v: string) => {
    if (!ctx) return;
    if (v === "") {
      ctx.onChange(facetKey.value, new FacetState({ set: false, selected: "" }));
      return;
    }
    ctx.onChange(facetKey.value, new FacetState({ set: true, selected: v }));
  },
});

function pickRadio(label: string) {
  if (!ctx) return;
  if (state.value.set && state.value.selected === label) {
    ctx.onChange(facetKey.value, new FacetState({ set: false, selected: "" }));
    return;
  }
  ctx.onChange(facetKey.value, new FacetState({ set: true, selected: label }));
}

function clear() {
  if (!ctx) return;
  ctx.onChange(facetKey.value, new FacetState({ set: false, selected: "" }));
}

const unavailableMessage = computed(() => {
  if (!ctx) return t("facet.field.unavailable");
  if (!facetKey.value) return t("facet.field.missing_binding");
  if (!facet.value) return t("facet.field.unknown_facet", { key: facetKey.value });
  return "";
});
</script>

<template>
  <div v-if="unavailableMessage" class="facet-field-unavailable muted small">
    {{ unavailableMessage }}
  </div>

  <div v-else-if="presentation === 'dropdown'" class="facet-field facet-field--dropdown">
    <SelectField
      v-model="dropdownValue"
      :options="selectOptions"
      :placeholder="t('facet.field.placeholder')"
    />
    <button
      v-if="state.set"
      type="button"
      class="tool-btn small facet-field-clear"
      :title="t('facet.field.clear')"
      @click="clear"
    >
      ×
    </button>
  </div>

  <div v-else class="facet-field facet-field--radio">
    <label
      v-for="o in facet!.options"
      :key="o.label"
      class="facet-field-radio-cell"
    >
      <input
        type="radio"
        :name="facetKey"
        :value="o.label"
        :checked="state.set && state.selected === o.label"
        @change="pickRadio(o.label)"
      />
      <FacetIcon
        class="facet-field-radio-icon"
        :class="`expr-text-${o.color}`"
        :icon="facet!.icon"
      />
      <span class="facet-field-radio-label">{{ o.label }}</span>
    </label>
    <button
      v-if="state.set"
      type="button"
      class="tool-btn small facet-field-clear"
      :title="t('facet.field.clear')"
      @click="clear"
    >
      {{ t('facet.field.clear') }}
    </button>
  </div>
</template>
