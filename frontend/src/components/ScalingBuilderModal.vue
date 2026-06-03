<script setup lang="ts">
/*
 * ScalingBuilderModal - authors the weighting for ONE facet: a reusable
 * per-option factor map. The source is locked to the facet it is opened from
 * (each facet owns a single weighting), so there is no source picker; you name
 * it and give each of the facet's options a factor, plus a default for unset
 * records. The expression engine reads it as S["name"] and the Statistical
 * Engine references it through the DSL scale "<name>" clause.
 *
 * Output is a top-level Scaling whose source is the facet.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ScrollList from "./ScrollList.vue";
import { TextField } from "./fields";
import { Scaling, StatSource } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Facet } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  /** The facet this weighting belongs to (its options are the weight keys). */
  facet: Facet | null;
  /** The existing weighting for this facet, or null to compose a new one. */
  initial: Scaling | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", scaling: Scaling): void;
  (e: "remove"): void;
}>();

const { t } = useI18n();

const name = ref("");
const label = ref("");
const factorByValue = ref<Record<string, number>>({});
const defaultFactor = ref(1);

// The weight keys are the facet's option labels (the value the engine carries
// per form is the selected option's label).
const choices = computed(() =>
  (props.facet?.options ?? [])
    .map((o) => o.label)
    .filter((l) => l !== "")
    .map((l) => ({ value: l, label: l })),
);

function factorOf(value: string): string {
  const v = factorByValue.value[value];
  return v === undefined ? "" : String(v);
}

function setFactor(value: string, raw: string) {
  const n = parseFloat(raw);
  factorByValue.value = { ...factorByValue.value, [value]: Number.isFinite(n) ? n : 0 };
}

function setDefault(raw: string) {
  const n = parseFloat(raw);
  defaultFactor.value = Number.isFinite(n) ? n : 0;
}

const isEditing = computed(() => props.initial !== null);
const canApply = computed(() => name.value.trim() !== "" && !!props.facet && choices.value.length > 0);

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    const sc = props.initial ?? null;
    if (sc) {
      name.value = sc.name;
      label.value = sc.label || "";
      defaultFactor.value = sc.default;
      const next: Record<string, number> = {};
      const preset: Record<string, number> = {};
      for (const w of sc.weights ?? []) preset[w.label] = w.factor;
      for (const c of choices.value) next[c.value] = c.value in preset ? preset[c.value] : sc.default;
      factorByValue.value = next;
    } else {
      // New weighting: default its name to the facet key so S["<facet>"] is the
      // memorable reference, and seed every option to the default factor.
      name.value = props.facet?.key ?? "";
      label.value = "";
      defaultFactor.value = 1;
      const next: Record<string, number> = {};
      for (const c of choices.value) next[c.value] = 1;
      factorByValue.value = next;
    }
  },
  { immediate: true },
);

function onApply() {
  if (!props.facet) return;
  const weights = choices.value.map((c) => ({ label: c.value, factor: factorByValue.value[c.value] ?? defaultFactor.value }));
  emit("apply", new Scaling({
    name: name.value.trim(),
    label: label.value.trim(),
    source: new StatSource({ kind: "facet", key: props.facet.key, column: "" }),
    weights,
    default: defaultFactor.value,
  }));
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.scaling_builder.title')"
    width="560px"
    @close="emit('close')"
  >
    <p class="muted small stat-builder-hint">
      {{ t('workspace.templates.scaling_builder.intro') }}
    </p>
    <p v-if="!facet || choices.length === 0" class="muted small stat-builder-empty">
      {{ t('workspace.templates.scaling_builder.no_options') }}
    </p>

    <div v-else class="stat-builder-form">
      <div class="stat-builder-ident">
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.name') }}</span>
          <TextField v-model="name" placeholder="fcdm-urgency" />
        </label>
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.label') }}</span>
          <TextField v-model="label" />
        </label>
      </div>

      <div class="stat-builder-blockedit">
        <div class="stat-block-group">{{ t('workspace.templates.scaling_builder.source') }}</div>
        <p class="muted small stat-builder-hint">
          {{ t('workspace.templates.scaling_builder.source_locked', [facet.key]) }}
        </p>

        <div class="stat-block-group">{{ t('workspace.templates.scaling_builder.weights') }}</div>
        <p class="muted small stat-builder-hint">
          {{ t('workspace.templates.scaling_builder.weights_hint') }}
        </p>
        <ScrollList max-height="15rem" class="stat-scaling-weights">
          <div v-for="c in choices" :key="c.value" class="stat-scaling-row">
            <span class="stat-scaling-option">{{ c.label }}</span>
            <TextField
              type="number"
              lazy
              :model-value="factorOf(c.value)"
              class="stat-scaling-factor"
              @update:model-value="(v: string) => setFactor(c.value, v)"
            />
          </div>
        </ScrollList>

        <label class="stat-builder-field stat-scaling-default">
          <span class="stat-builder-field-label">{{ t('workspace.templates.scaling_builder.default') }}</span>
          <TextField
            type="number"
            lazy
            :model-value="String(defaultFactor)"
            @update:model-value="setDefault"
          />
        </label>
      </div>
    </div>

    <template #footer>
      <button
        v-if="isEditing"
        class="tool-btn danger"
        type="button"
        @click="emit('remove')"
      >
        {{ t('workspace.templates.scalings.remove') }}
      </button>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canApply" @click="onApply">
        {{ t('workspace.templates.stat_builder.apply') }}
      </button>
    </template>
  </Modal>
</template>
