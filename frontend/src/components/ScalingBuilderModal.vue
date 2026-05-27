<script setup lang="ts">
/*
 * ScalingBuilderModal - authors one scaling statistical object: a reusable
 * weighting that other objects reference by name (their DSL `scale "<name>"`
 * clause). It picks a per-form categorical source (a facet, or a dropdown /
 * radio field) and assigns each option a numeric factor, plus a default for
 * unlisted options. A weighted count()/records() then sums these factors per
 * form instead of adding 1 (e.g. weight applications by fcdm coverage so low
 * coverage counts heavier). Compact single pane: the option set is closed and
 * short, so a flat label/factor list beats a master-detail layout.
 *
 * Output is a Statistic carrying a scaling spec (no DSL of its own).
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SelectField, TextField } from "./fields";
import type { Field, Facet, Statistic } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  fields: Field[];
  facets: Facet[];
  /** The scaling being edited, or null to compose a new one. */
  initial: Statistic | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", stat: Statistic): void;
}>();

const { t } = useI18n();

const name = ref("");
const label = ref("");
const sourceKey = ref(""); // stable select value: "<kind>|<fieldKey>"
const factorByValue = ref<Record<string, number>>({});
const defaultFactor = ref(1);

// ── Per-form sources: facets + dropdown/radio fields ────────────────
// A facet filter matches the selected option's label; a dropdown/radio field
// stores the option's value. Those stored strings are exactly what the engine
// carries per form, so they are the weight keys.
interface ScaleSource {
  key: string;
  kind: "field" | "facet";
  fieldKey: string;
  label: string;
  choices: { value: string; label: string }[];
}

const sources = computed<ScaleSource[]>(() => {
  const out: ScaleSource[] = [];
  for (const f of props.fields ?? []) {
    if (f.type !== "dropdown" && f.type !== "radio") continue;
    const choices = ((f.options ?? []) as Array<Record<string, unknown>>)
      .map((o) => ({ value: String(o?.value ?? ""), label: String(o?.label ?? o?.value ?? "") }))
      .filter((c) => c.value !== "");
    if (choices.length === 0) continue;
    out.push({ key: `field|${f.key}`, kind: "field", fieldKey: f.key, label: f.label || f.key, choices });
  }
  for (const fc of props.facets ?? []) {
    const choices = (fc.options ?? [])
      .map((o) => o.label)
      .filter((l) => l !== "")
      .map((l) => ({ value: l, label: l }));
    if (choices.length === 0) continue;
    out.push({ key: `facet|${fc.key}`, kind: "facet", fieldKey: fc.key, label: fc.key, choices });
  }
  return out;
});

const sourceOptions = computed(() => sources.value.map((s) => ({ value: s.key, label: s.label })));
const currentSource = computed(() => sources.value.find((s) => s.key === sourceKey.value) ?? null);

function setSource(key: string, preset?: Record<string, number>) {
  sourceKey.value = key;
  const s = sources.value.find((x) => x.key === key);
  const next: Record<string, number> = {};
  for (const c of s?.choices ?? []) {
    next[c.value] = preset && c.value in preset ? preset[c.value] : defaultFactor.value;
  }
  factorByValue.value = next;
}

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

const hasSources = computed(() => sources.value.length > 0);
const canApply = computed(() => name.value.trim() !== "" && currentSource.value !== null);

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    const sc = props.initial?.scaling ?? null;
    if (sc) {
      name.value = props.initial!.name;
      label.value = props.initial!.label || "";
      defaultFactor.value = sc.default;
      const preset: Record<string, number> = {};
      for (const w of sc.weights ?? []) preset[w.label] = w.factor;
      setSource(`${sc.source.kind}|${sc.source.key}`, preset);
    } else {
      name.value = "";
      label.value = "";
      defaultFactor.value = 1;
      sourceKey.value = "";
      factorByValue.value = {};
    }
  },
  { immediate: true },
);

function onApply() {
  const s = currentSource.value;
  if (!s) return;
  const weights = s.choices.map((c) => ({ label: c.value, factor: factorByValue.value[c.value] ?? defaultFactor.value }));
  emit("apply", {
    name: name.value.trim(),
    label: label.value.trim(),
    dsl: "",
    scaling: {
      source: { kind: s.kind, key: s.fieldKey, column: "" },
      weights,
      default: defaultFactor.value,
    },
  } as unknown as Statistic);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.scaling_builder.title')"
    width="560px"
    scroll
    @close="emit('close')"
  >
    <p class="muted small stat-builder-hint">
      {{ t('workspace.templates.scaling_builder.intro') }}
    </p>
    <p v-if="!hasSources" class="muted small stat-builder-empty">
      {{ t('workspace.templates.scaling_builder.no_sources') }}
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
        <SelectField
          :model-value="sourceKey"
          :options="sourceOptions"
          :placeholder="t('workspace.templates.scaling_builder.pick_source')"
          @update:model-value="(v: string) => setSource(v)"
        />

        <template v-if="currentSource">
          <div class="stat-block-group">{{ t('workspace.templates.scaling_builder.weights') }}</div>
          <p class="muted small stat-builder-hint">
            {{ t('workspace.templates.scaling_builder.weights_hint') }}
          </p>
          <div class="stat-scaling-weights">
            <div v-for="c in currentSource.choices" :key="c.value" class="stat-scaling-row">
              <span class="stat-scaling-option">{{ c.label }}</span>
              <TextField
                type="number"
                lazy
                :model-value="factorOf(c.value)"
                class="stat-scaling-factor"
                @update:model-value="(v: string) => setFactor(c.value, v)"
              />
            </div>
          </div>

          <label class="stat-builder-field stat-scaling-default">
            <span class="stat-builder-field-label">{{ t('workspace.templates.scaling_builder.default') }}</span>
            <TextField
              type="number"
              lazy
              :model-value="String(defaultFactor)"
              @update:model-value="setDefault"
            />
          </label>
        </template>
      </div>
    </div>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canApply" @click="onApply">
        {{ t('workspace.templates.stat_builder.apply') }}
      </button>
    </template>
  </Modal>
</template>
