<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import TextField from "./TextField.vue";
import type { SubRowVariant } from "./OptionsEditor.vue";

const { t } = useI18n();

// Generic sub-option editor - a structured editor for the canonical
// "value:label|value:label" sub-option string. Two modes:
//
//   1. Fixed entries (variant.entries[]): one row per entry, the
//      value column is locked to entry.value and the user only edits
//      the label. Used by bool columns so the data layer always sees
//      "true" / "false" no matter what label the user types.
//
//   2. Free-form pairs: the user adds N rows of {value, label} with
//      add/remove buttons, capped by variant.maxEntries when set.
//      Used by dropdown columns.
//
// The model on the wire stays a pipe-delimited "value:label|..."
// string so the form-side parseChoices works unchanged.

type Pair = { value: string; label: string };

const props = defineProps<{
  variant: SubRowVariant;
}>();

const model = defineModel<string>({ default: "" });

function parsePairs(raw: string): Pair[] {
  if (!raw) return [];
  return raw
    .split("|")
    .map((piece) => piece.trim())
    .filter(Boolean)
    .map((piece) => {
      const idx = piece.indexOf(":");
      if (idx === -1) return { value: piece, label: piece };
      return {
        value: piece.slice(0, idx).trim(),
        label: piece.slice(idx + 1).trim(),
      };
    });
}

function serializePairs(pairs: Pair[]): string {
  return pairs
    .map((p) => {
      const v = p.value.trim();
      const l = p.label.trim();
      if (!v && !l) return "";
      if (l === "" || l === v) return v;
      return `${v}:${l}`;
    })
    .filter(Boolean)
    .join("|");
}

// `pairs` is local state, not a computed off `model`. Why: a freshly
// added pair starts {value:"", label:""}, which serializePairs collapses
// to "" - round-tripping that through the model string drops the row
// before the user can type into it (the "+ doesn't work" bug). We
// hydrate from `model` on first run and on external resets, but the
// component owns the live editing state from then on.
const pairs = ref<Pair[]>(parsePairs(model.value));
watch(
  () => model.value,
  (v) => {
    // External write - only re-hydrate if `model` no longer reflects
    // what we last serialized, otherwise our own commits would clobber
    // half-typed rows.
    if (serializePairs(pairs.value) !== v) {
      pairs.value = parsePairs(v);
    }
  },
);

// Fixed-entries mode: derived rows that pad/replace the parsed pairs
// so each entry's value stays locked to the variant config.
const fixedPairs = computed<Pair[]>(() => {
  const entries = props.variant.entries;
  if (!entries) return [];
  return entries.map((e) => {
    const match = pairs.value.find((p) => p.value === e.value);
    return { value: e.value, label: match?.label ?? "" };
  });
});

const isScalar = computed(() => !!props.variant.scalar);
const isFixed = computed(() => !!(props.variant.entries && props.variant.entries.length > 0));
const max = computed(() => props.variant.maxEntries ?? 0);
const canAdd = computed(() => max.value <= 0 || pairs.value.length < max.value);

function commit(next: Pair[]): void {
  pairs.value = next;
  model.value = serializePairs(next);
}

function updatePair(idx: number, field: "value" | "label", value: string): void {
  const next = pairs.value.slice();
  while (next.length <= idx) next.push({ value: "", label: "" });
  next[idx] = { ...next[idx], [field]: value };
  commit(next);
}

function updateFixedLabel(entryValue: string, label: string): void {
  const next = pairs.value.slice();
  const i = next.findIndex((p) => p.value === entryValue);
  if (i === -1) {
    next.push({ value: entryValue, label });
  } else {
    next[i] = { value: entryValue, label };
  }
  commit(next);
}

function addPair(): void {
  if (!canAdd.value) return;
  commit([...pairs.value, { value: "", label: "" }]);
}

function removePair(idx: number): void {
  commit(pairs.value.filter((_, i) => i !== idx));
}
</script>

<template>
  <div class="options-subrow-widget">
    <span v-if="variant.labelKey" class="options-subrow-label small">
      {{ t(variant.labelKey) }}
    </span>

    <div class="options-subrow-pairs">
      <!-- Scalar mode: a single raw value (e.g. a number column's step). -->
      <div v-if="isScalar" class="options-subrow-pair">
        <TextField
          :model-value="model"
          @update:model-value="(v) => (model = v)"
          :placeholder="variant.defaultValue ?? ''"
          class="options-subrow-input"
        />
      </div>

      <!-- Fixed-entries mode: one row per entry, value locked. -->
      <template v-else-if="isFixed && variant.entries">
        <div
          v-for="(entry, ei) in variant.entries"
          :key="`fixed-${ei}`"
          class="options-subrow-pair"
        >
          <span class="options-subrow-entry-label small">{{ t(entry.labelKey) }}</span>
          <span class="options-subrow-locked-value mono">{{ entry.value }}</span>
          <TextField
            :model-value="fixedPairs[ei]?.label ?? ''"
            @update:model-value="(v) => updateFixedLabel(entry.value, v)"
            :placeholder="entry.placeholderKey ? t(entry.placeholderKey) : ''"
            class="options-subrow-input"
          />
        </div>
      </template>

      <!-- Free-form pairs mode: add/remove value + label. -->
      <template v-else>
        <div
          v-for="(pair, pi) in pairs"
          :key="`pair-${pi}`"
          class="options-subrow-pair"
        >
          <TextField
            :model-value="pair.value"
            @update:model-value="(v) => updatePair(pi, 'value', v)"
            :placeholder="t('workspace.templates.options.choice_value')"
            class="options-subrow-input options-subrow-input-value"
          />
          <TextField
            :model-value="pair.label"
            @update:model-value="(v) => updatePair(pi, 'label', v)"
            :placeholder="t('workspace.templates.options.choice_label')"
            class="options-subrow-input"
          />
          <button
            type="button"
            class="btn-ghost-icon"
            @click="removePair(pi)"
            :title="t('workspace.templates.options.remove_choice')"
            :aria-label="t('workspace.templates.options.remove_choice')"
          >−</button>
        </div>
        <button
          type="button"
          class="btn-ghost-block"
          :disabled="!canAdd"
          @click="addPair"
          :title="t('workspace.templates.options.add_choice')"
          :aria-label="t('workspace.templates.options.add_choice')"
        >+</button>
      </template>
    </div>
  </div>
</template>
