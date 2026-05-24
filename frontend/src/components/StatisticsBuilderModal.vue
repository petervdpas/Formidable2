<script setup lang="ts">
/*
 * StatisticsBuilderModal - visual builder for one of a template's named
 * statistical objects (the Statistical Engine). The author composes
 * measures (cell values) over dimensions (axes); the result is a DSL
 * string compiled/parsed by the backend (stat.Compile / stat.Parse via
 * the Stat service) - backend is the source of truth for the grammar so
 * the dialog can't drift from the engine. Sources are the template's
 * use_in_statistics fields (table columns via statistics_columns) and its
 * facets. Rendering/chart type is NOT decided here - the object is data.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SelectField, TextField } from "./fields";
import { Service as StatSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import {
  MeasureOp,
  Bin,
  type MeasureOpDescriptor,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat/models";
import type {
  Field,
  Facet,
  Statistic,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { backendErrMessage } from "../utils/backendError";

// Plain shapes mirroring the generated stat models (PascalCase fields);
// passed straight to CompileDSL and received from ParseDSL.
interface SourceRef {
  Kind: string;
  Key: string;
  Column: string;
}
interface Measure {
  Op: string;
  Source: SourceRef | null;
  Arg: number | null;
}
interface Dimension {
  Source: SourceRef;
  Bin: string;
  Top: number;
}
interface StatConfig {
  Measures: Measure[];
  Dimensions: Dimension[];
}

const props = defineProps<{
  open: boolean;
  fields: Field[];
  facets: Facet[];
  /** The statistic being edited, or null to compose a new one. */
  initial: Statistic | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "apply", stat: Statistic): void;
}>();

const { t } = useI18n();

const name = ref("");
const label = ref("");
const config = ref<StatConfig>({ Measures: [], Dimensions: [] });
const dslPreview = ref("");
const compileError = ref("");
const parseWarn = ref(false);

// ── Sources derived from the template ───────────────────────────────
interface SourceOpt {
  key: string; // stable select value
  ref: SourceRef;
  label: string;
  numeric: boolean;
  date: boolean;
  text: boolean; // a free-text field: high-cardinality, prefill a top-N cap
}

function srcKey(s: SourceRef): string {
  return `${s.Kind}|${s.Key}|${s.Column || ""}`;
}

const sources = computed<SourceOpt[]>(() => {
  const out: SourceOpt[] = [];
  for (const f of props.fields ?? []) {
    if (!f.use_in_statistics) continue;
    const flabel = f.label || f.key;
    if (f.type === "table") {
      const cols = (f.statistics_columns ?? []) as string[];
      const opts = (f.options ?? []) as Array<Record<string, unknown>>;
      for (const colKey of cols) {
        const o = opts.find((x) => String(x?.value ?? "") === colKey);
        const ctype = String(o?.type ?? "string");
        const clabel = String(o?.label ?? colKey);
        const ref: SourceRef = { Kind: "field", Key: f.key, Column: colKey };
        out.push({
          key: srcKey(ref),
          ref,
          label: `${flabel} / ${clabel}`,
          numeric: ctype === "number",
          date: ctype === "date",
          text: false,
        });
      }
    } else {
      const ref: SourceRef = { Kind: "field", Key: f.key, Column: "" };
      out.push({
        key: srcKey(ref),
        ref,
        label: flabel,
        numeric: f.type === "number" || f.type === "range",
        date: f.type === "date",
        text: f.type === "text",
      });
    }
  }
  for (const fc of props.facets ?? []) {
    const ref: SourceRef = { Kind: "facet", Key: fc.key, Column: "" };
    out.push({ key: srcKey(ref), ref, label: fc.key, numeric: false, date: false, text: false });
  }
  return out;
});

const sourceByKey = computed<Record<string, SourceOpt>>(() => {
  const m: Record<string, SourceOpt> = {};
  for (const s of sources.value) m[s.key] = s;
  return m;
});

const allSourceOptions = computed(() => sources.value.map((s) => ({ value: s.key, label: s.label })));
const numericSourceOptions = computed(() =>
  sources.value.filter((s) => s.numeric).map((s) => ({ value: s.key, label: s.label })),
);

// Op / Bin catalogs come from the backend (Stat.BuilderMeasureOps /
// BuilderBins) so the engine owns the vocabulary AND the input rules
// (which ops need a source / an argument). The UI only owns wording, via
// the explicit i18n-key maps below (no interpolated keys).
const measureOps = ref<MeasureOpDescriptor[]>([]);
const bins = ref<string[]>([]);

const opRule = computed<Record<string, { needsSource: boolean; needsArg: boolean }>>(() => {
  const m: Record<string, { needsSource: boolean; needsArg: boolean }> = {};
  for (const d of measureOps.value) m[d.op] = { needsSource: d.needs_source, needsArg: d.needs_arg };
  return m;
});
function opNeedsSource(op: string): boolean {
  return opRule.value[op]?.needsSource ?? false;
}
function opNeedsArg(op: string): boolean {
  return opRule.value[op]?.needsArg ?? false;
}

const opLabelKeys: Record<string, string> = {
  count: "workspace.templates.stat_builder.op.count",
  sum: "workspace.templates.stat_builder.op.sum",
  avg: "workspace.templates.stat_builder.op.avg",
  min: "workspace.templates.stat_builder.op.min",
  max: "workspace.templates.stat_builder.op.max",
  median: "workspace.templates.stat_builder.op.median",
  stddev: "workspace.templates.stat_builder.op.stddev",
  percentile: "workspace.templates.stat_builder.op.percentile",
};
const measureOpOptions = computed(() =>
  measureOps.value.map((d) => ({
    value: d.op as string,
    label: opLabelKeys[d.op] ? t(opLabelKeys[d.op]) : (d.op as string),
  })),
);

const binLabelKeys: Record<string, string> = {
  "": "workspace.templates.stat_builder.bin.none",
  year: "workspace.templates.stat_builder.bin.year",
  month: "workspace.templates.stat_builder.bin.month",
  day: "workspace.templates.stat_builder.bin.day",
};
const binOptions = computed(() =>
  bins.value.map((b) => ({ value: b, label: binLabelKeys[b] ? t(binLabelKeys[b]) : b })),
);

function dimIsDate(d: Dimension): boolean {
  return !!sourceByKey.value[srcKey(d.Source)]?.date;
}

// ── Measure ops ─────────────────────────────────────────────────────
function addMeasure() {
  config.value.Measures = [...config.value.Measures, { Op: MeasureOp.OpCount, Source: null, Arg: null }];
}
function removeMeasure(i: number) {
  config.value.Measures = config.value.Measures.filter((_, j) => j !== i);
}
function setMeasureOp(i: number, op: string) {
  const m: Measure = { ...config.value.Measures[i], Op: op };
  if (!opNeedsSource(op)) {
    m.Source = null;
    m.Arg = null;
  } else {
    if (!m.Source) {
      const first = sources.value.find((s) => s.numeric);
      m.Source = first ? { ...first.ref } : null;
    }
    m.Arg = opNeedsArg(op) ? (m.Arg ?? 90) : null;
  }
  config.value.Measures = config.value.Measures.map((x, j) => (j === i ? m : x));
}
function setMeasureSource(i: number, key: string) {
  const s = sourceByKey.value[key];
  if (!s) return;
  config.value.Measures = config.value.Measures.map((x, j) =>
    j === i ? { ...x, Source: { ...s.ref } } : x,
  );
}
function setMeasureArg(i: number, v: string) {
  const n = Number(v);
  config.value.Measures = config.value.Measures.map((x, j) =>
    j === i ? { ...x, Arg: Number.isNaN(n) ? null : n } : x,
  );
}

// ── Dimension ops ───────────────────────────────────────────────────
function addDimension() {
  const first = sources.value[0];
  if (!first) return;
  config.value.Dimensions = [
    ...config.value.Dimensions,
    { Source: { ...first.ref }, Bin: Bin.BinNone, Top: first.text ? 10 : 0 },
  ];
}
function removeDimension(i: number) {
  config.value.Dimensions = config.value.Dimensions.filter((_, j) => j !== i);
}
function setDimensionSource(i: number, key: string) {
  const s = sourceByKey.value[key];
  if (!s) return;
  config.value.Dimensions = config.value.Dimensions.map((x, j) => {
    if (j !== i) return x;
    // Prefill a top-10 cap when switching to a (high-cardinality) text
    // source, unless a cap is already set. top-N is generic - any
    // dimension can carry one; only the prefill is text-specific.
    const top = s.text && (x.Top ?? 0) === 0 ? 10 : x.Top ?? 0;
    return { Source: { ...s.ref }, Bin: s.date ? x.Bin : Bin.BinNone, Top: top };
  });
}
function setDimensionBin(i: number, bin: string) {
  config.value.Dimensions = config.value.Dimensions.map((x, j) => (j === i ? { ...x, Bin: bin } : x));
}
function setDimensionTop(i: number, v: string) {
  let n = Math.floor(Number(v));
  if (!Number.isFinite(n) || n <= 0) {
    n = 0; // no cap
  } else {
    n = Math.min(20, Math.max(1, n));
  }
  config.value.Dimensions = config.value.Dimensions.map((x, j) => (j === i ? { ...x, Top: n } : x));
}

// ── DSL compile preview ─────────────────────────────────────────────
async function recompile() {
  if (config.value.Measures.length === 0) {
    dslPreview.value = "";
    compileError.value = "";
    return;
  }
  try {
    dslPreview.value = await StatSvc.CompileDSL(config.value as never);
    compileError.value = "";
  } catch (err) {
    compileError.value = backendErrMessage(err);
  }
}
watch(config, () => void recompile(), { deep: true });

// ── Open: load initial or start fresh ───────────────────────────────
function freshConfig(): StatConfig {
  return { Measures: [{ Op: MeasureOp.OpCount, Source: null, Arg: null }], Dimensions: [] };
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    parseWarn.value = false;
    compileError.value = "";
    dslPreview.value = "";
    if (measureOps.value.length === 0) {
      const [ops, bs] = await Promise.all([StatSvc.BuilderMeasureOps(), StatSvc.BuilderBins()]);
      measureOps.value = ops;
      bins.value = bs;
    }
    if (props.initial) {
      name.value = props.initial.name;
      label.value = props.initial.label || "";
      const dsl = (props.initial.dsl || "").trim();
      if (dsl) {
        try {
          const parsed = await StatSvc.ParseDSL(dsl);
          config.value = {
            Measures: (parsed.Measures ?? []) as Measure[],
            Dimensions: (parsed.Dimensions ?? []) as Dimension[],
          };
        } catch {
          config.value = freshConfig();
          parseWarn.value = true;
        }
      } else {
        config.value = freshConfig();
      }
    } else {
      name.value = "";
      label.value = "";
      config.value = freshConfig();
    }
    await recompile();
  },
  { immediate: true },
);

const hasSources = computed(() => sources.value.length > 0);
const canApply = computed(
  () => name.value.trim() !== "" && config.value.Measures.length > 0 && !compileError.value,
);

async function onApply() {
  await recompile();
  if (compileError.value) return;
  emit("apply", {
    name: name.value.trim(),
    label: label.value.trim(),
    dsl: dslPreview.value,
  } as Statistic);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.templates.stat_builder.title')"
    width="760px"
    @close="emit('close')"
  >
    <p v-if="parseWarn" class="stat-builder-warn small">
      {{ t('workspace.templates.stat_builder.parse_failed') }}
    </p>
    <p v-if="!hasSources" class="muted small stat-builder-empty">
      {{ t('workspace.templates.stat_builder.no_sources') }}
    </p>

    <div class="stat-builder-form">
      <div class="stat-builder-ident">
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.name') }}</span>
          <TextField v-model="name" placeholder="by-status" />
        </label>
        <label class="stat-builder-field">
          <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.label') }}</span>
          <TextField v-model="label" />
        </label>
      </div>

      <!-- MEASURES -->
      <fieldset class="stat-builder-fieldset">
        <legend>{{ t('workspace.templates.stat_builder.measures') }}</legend>
        <div class="options-editor">
          <div class="options-rows">
            <div v-for="(m, i) in config.Measures" :key="`m${i}`" class="options-row">
              <SelectField
                :model-value="m.Op"
                :options="measureOpOptions"
                class="options-cell"
                @update:model-value="(v: string) => setMeasureOp(i, v)"
              />
              <SelectField
                v-if="opNeedsSource(m.Op)"
                :model-value="m.Source ? srcKey(m.Source) : ''"
                :options="numericSourceOptions"
                class="options-cell"
                @update:model-value="(v: string) => setMeasureSource(i, v)"
              />
              <TextField
                v-if="opNeedsArg(m.Op)"
                type="number"
                lazy
                :model-value="String(m.Arg ?? 90)"
                class="stat-builder-arg"
                @update:model-value="(v: string) => setMeasureArg(i, v)"
              />
              <button
                type="button"
                class="btn-ghost-icon"
                :title="t('workspace.templates.stat_builder.remove')"
                @click="removeMeasure(i)"
              >−</button>
            </div>
          </div>
          <button
            type="button"
            class="btn-ghost-block"
            :title="t('workspace.templates.stat_builder.add_measure')"
            @click="addMeasure"
          >+ {{ t('workspace.templates.stat_builder.add_measure') }}</button>
        </div>
      </fieldset>

      <!-- DIMENSIONS -->
      <fieldset class="stat-builder-fieldset">
        <legend>{{ t('workspace.templates.stat_builder.dimensions') }}</legend>
        <p class="muted small stat-builder-hint">
          {{ t('workspace.templates.stat_builder.dimensions_hint') }}
        </p>
        <div class="options-editor">
          <div class="options-rows">
            <div v-for="(d, i) in config.Dimensions" :key="`d${i}`" class="options-row">
              <SelectField
                :model-value="srcKey(d.Source)"
                :options="allSourceOptions"
                class="options-cell"
                @update:model-value="(v: string) => setDimensionSource(i, v)"
              />
              <SelectField
                v-if="dimIsDate(d)"
                :model-value="d.Bin"
                :options="binOptions"
                class="options-cell"
                @update:model-value="(v: string) => setDimensionBin(i, v)"
              />
              <TextField
                type="number"
                lazy
                :model-value="d.Top ? String(d.Top) : ''"
                class="stat-builder-arg"
                :placeholder="t('workspace.templates.stat_builder.top')"
                @update:model-value="(v: string) => setDimensionTop(i, v)"
              />
              <button
                type="button"
                class="btn-ghost-icon"
                :title="t('workspace.templates.stat_builder.remove')"
                @click="removeDimension(i)"
              >−</button>
            </div>
          </div>
          <button
            type="button"
            class="btn-ghost-block"
            :disabled="!hasSources"
            :title="t('workspace.templates.stat_builder.add_dimension')"
            @click="addDimension"
          >+ {{ t('workspace.templates.stat_builder.add_dimension') }}</button>
        </div>
      </fieldset>

      <!-- PREVIEW -->
      <div class="stat-builder-preview">
        <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.preview') }}</span>
        <code v-if="dslPreview" class="stat-builder-dsl">{{ dslPreview }}</code>
        <code v-else class="stat-builder-dsl muted">{{ t('workspace.templates.stat_builder.preview_empty') }}</code>
        <p v-if="compileError" class="stat-builder-error small">{{ compileError }}</p>
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
