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
import draggable from "vuedraggable";
import Modal from "./Modal.vue";
import { SelectField, TextField } from "./fields";
import { Service as StatSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";
import {
  MeasureOp,
  Bin,
  FilterOp,
  type MeasureOpDescriptor,
  type FilterOpDescriptor,
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
interface Filter {
  Source: SourceRef;
  Op: string;
  Value: string;
}
interface StatConfig {
  Measures: Measure[];
  Dimensions: Dimension[];
  Filters: Filter[];
  Percent: string; // "" = distribution (default)
  Scale: string; // "" = unweighted; else a scaling object's name
}

const props = defineProps<{
  open: boolean;
  /** Active template filename, so the live preview can evaluate the DSL. */
  template: string;
  fields: Field[];
  facets: Facet[];
  /** Scaling objects on the template, for the optional weighting picker. */
  scalings: Statistic[];
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
const config = ref<StatConfig>({ Measures: [], Dimensions: [], Filters: [], Percent: "", Scale: "" });
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
  // Closed value set (facet option labels). When present, a filter on this
  // source offers a dropdown instead of free text, so the author can't mistype
  // a value that exists nowhere in the data.
  choices?: { value: string; label: string }[];
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
      // dropdown/radio fields store the chosen option's `value`, so that's
      // the closed set a filter should compare against.
      let choices: { value: string; label: string }[] | undefined;
      if (f.type === "dropdown" || f.type === "radio") {
        choices = ((f.options ?? []) as Array<Record<string, unknown>>)
          .map((o) => ({ value: String(o?.value ?? ""), label: String(o?.label ?? o?.value ?? "") }))
          .filter((c) => c.value !== "");
      }
      out.push({
        key: srcKey(ref),
        ref,
        label: flabel,
        numeric: f.type === "number" || f.type === "range",
        date: f.type === "date",
        text: f.type === "text",
        choices,
      });
    }
  }
  for (const fc of props.facets ?? []) {
    const ref: SourceRef = { Kind: "facet", Key: fc.key, Column: "" };
    // A facet filter matches the selected option's label, so the closed set
    // of choices is exactly the option labels.
    const choices = (fc.options ?? [])
      .map((o) => o.label)
      .filter((l) => l !== "")
      .map((l) => ({ value: l, label: l }));
    out.push({ key: srcKey(ref), ref, label: fc.key, numeric: false, date: false, text: false, choices });
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
  records: "workspace.templates.stat_builder.op.records",
  sum: "workspace.templates.stat_builder.op.sum",
  avg: "workspace.templates.stat_builder.op.avg",
  min: "workspace.templates.stat_builder.op.min",
  max: "workspace.templates.stat_builder.op.max",
  median: "workspace.templates.stat_builder.op.median",
  stddev: "workspace.templates.stat_builder.op.stddev",
  percentile: "workspace.templates.stat_builder.op.percentile",
};
const measureOpOptions = computed(() => {
  // Hide source-needing ops (sum/avg/...) when the template has no numeric
  // source to feed them - count() is the only sensible measure then.
  const haveNumeric = numericSourceOptions.value.length > 0;
  return measureOps.value
    .filter((d) => haveNumeric || !d.needs_source)
    .map((d) => ({
      value: d.op as string,
      label: opLabelKeys[d.op] ? t(opLabelKeys[d.op]) : (d.op as string),
    }));
});

const binLabelKeys: Record<string, string> = {
  "": "workspace.templates.stat_builder.bin.none",
  year: "workspace.templates.stat_builder.bin.year",
  month: "workspace.templates.stat_builder.bin.month",
  day: "workspace.templates.stat_builder.bin.day",
};
const binOptions = computed(() =>
  bins.value.map((b) => ({ value: b, label: binLabelKeys[b] ? t(binLabelKeys[b]) : b })),
);

// Percentage base: which denominator the engine uses for each cell's pct.
// Catalog from the backend; the UI only labels it.
const percentBases = ref<string[]>([]);
const pctBaseLabelKeys: Record<string, string> = {
  distribution: "workspace.templates.stat_builder.pct.distribution",
  forms: "workspace.templates.stat_builder.pct.forms",
  none: "workspace.templates.stat_builder.pct.none",
};
const percentBaseOptions = computed(() =>
  percentBases.value.map((b) => ({ value: b, label: pctBaseLabelKeys[b] ? t(pctBaseLabelKeys[b]) : b })),
);
function setPercent(v: string) {
  config.value = { ...config.value, Percent: v };
}

// Scaling: an optional reusable weighting (a scaling object referenced by
// name). The picker lists the template's scaling objects plus "none". Weighting
// only changes count()/records() values, so it's a no-op on numeric-only
// measures, but the picker stays available either way.
const scaleOptions = computed(() => [
  { value: "", label: t("workspace.templates.stat_builder.scale.none") },
  ...(props.scalings ?? []).map((s) => ({ value: s.name, label: s.label || s.name })),
]);
const hasScalings = computed(() => (props.scalings ?? []).length > 0);
function setScale(v: string) {
  config.value = { ...config.value, Scale: v };
}

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
  // Percentile arg: 0..100. Empty / non-numeric snaps to a sensible 90.
  const trimmed = v.trim();
  let n = trimmed === "" ? 90 : Number(trimmed);
  if (!Number.isFinite(n)) n = 90;
  else n = Math.min(100, Math.max(0, n));
  config.value.Measures = config.value.Measures.map((x, j) =>
    j === i ? { ...x, Arg: n } : x,
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
    // A high-cardinality text source prefills a top-10 cap (keeping any
    // existing one); non-text sources (facets, dropdowns) have a small
    // known set, so switching to one clears the cap rather than leaving a
    // spurious top from the previous source. top-N stays generic - you can
    // re-add it on any dimension by hand.
    const top = s.text ? (x.Top > 0 ? x.Top : 10) : 0;
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

// ── Filter (where) ops ──────────────────────────────────────────────
// Operator catalog from the backend (op + whether the value is numeric).
const filterOps = ref<FilterOpDescriptor[]>([]);
const opNumeric = computed<Record<string, boolean>>(() => {
  const m: Record<string, boolean> = {};
  for (const d of filterOps.value) m[d.op] = d.numeric;
  return m;
});
function filterOpIsNumeric(op: string): boolean {
  return opNumeric.value[op] ?? false;
}

const filterOpLabelKeys: Record<string, string> = {
  eq: "workspace.templates.stat_builder.op_label.eq",
  ne: "workspace.templates.stat_builder.op_label.ne",
  lt: "workspace.templates.stat_builder.op_label.lt",
  le: "workspace.templates.stat_builder.op_label.le",
  gt: "workspace.templates.stat_builder.op_label.gt",
  ge: "workspace.templates.stat_builder.op_label.ge",
};
// Per-row op options: comparison ops only make sense on a numeric source;
// non-numeric sources (text, facet, dropdown) get equality only.
function filterOpOptionsFor(f: Filter) {
  const numericSrc = !!sourceByKey.value[srcKey(f.Source)]?.numeric;
  return filterOps.value
    .filter((d) => numericSrc || !d.numeric)
    .map((d) => ({ value: d.op as string, label: filterOpLabelKeys[d.op] ? t(filterOpLabelKeys[d.op]) : (d.op as string) }));
}

function addFilter() {
  const first = sources.value[0];
  if (!first) return;
  config.value.Filters = [...config.value.Filters, { Source: { ...first.ref }, Op: FilterOp.FilterEq, Value: "" }];
}
function removeFilter(i: number) {
  config.value.Filters = config.value.Filters.filter((_, j) => j !== i);
}
function setFilterSource(i: number, key: string) {
  const s = sourceByKey.value[key];
  if (!s) return;
  config.value.Filters = config.value.Filters.map((x, j) => {
    if (j !== i) return x;
    // A comparison op on a non-numeric source is invalid - fall back to eq.
    const op = !s.numeric && filterOpIsNumeric(x.Op) ? FilterOp.FilterEq : x.Op;
    // Switching to a closed-set source whose current value isn't a valid
    // choice: seed the first option so the dropdown never lands on a value
    // that matches nothing.
    let value = x.Value;
    if (s.choices && s.choices.length > 0 && !s.choices.some((c) => c.value === value)) {
      value = s.choices[0].value;
    }
    return { Source: { ...s.ref }, Op: op, Value: value };
  });
}

// Closed value set for the selected filter's source, or null for free text.
function filterChoicesFor(f: Filter): { value: string; label: string }[] | null {
  const c = sourceByKey.value[srcKey(f.Source)]?.choices;
  return c && c.length > 0 ? c : null;
}
function setFilterOp(i: number, op: string) {
  config.value.Filters = config.value.Filters.map((x, j) => (j === i ? { ...x, Op: op } : x));
}
function setFilterValue(i: number, v: string) {
  config.value.Filters = config.value.Filters.map((x, j) => (j === i ? { ...x, Value: v } : x));
}

// ── Block selection (master-detail) ─────────────────────────────────
// The left list is the statistic's outline (typed block chips); the right
// pane edits the one selected block. Keeps only one block's controls on
// screen at a time instead of stacking every section.
type BlockKind = "measure" | "dimension" | "filter" | "percent" | "scale";
const selected = ref<{ kind: BlockKind; index: number }>({ kind: "measure", index: 0 });

function selectBlock(kind: BlockKind, index: number) {
  selected.value = { kind, index };
}

// Stable dnd key per block object, kept off the data (a WeakMap, not an id
// field) so the StatConfig sent to compile stays clean. Reordering splices
// the same object references, so keys hold during a drag.
const keyMap = new WeakMap<object, string>();
let keySeq = 0;
function blockKey(o: object): string {
  let k = keyMap.get(o);
  if (!k) {
    k = `k${++keySeq}`;
    keyMap.set(o, k);
  }
  return k;
}
function isSelected(kind: BlockKind, index: number): boolean {
  return selected.value.kind === kind && selected.value.index === index;
}
// After a removal, fall back to the first measure (always present once one
// exists) or the always-present percentage block.
function selectSafe() {
  selected.value = config.value.Measures.length > 0
    ? { kind: "measure", index: 0 }
    : { kind: "percent", index: 0 };
}

const curMeasure = computed(() =>
  selected.value.kind === "measure" ? (config.value.Measures[selected.value.index] ?? null) : null,
);
const curDimension = computed(() =>
  selected.value.kind === "dimension" ? (config.value.Dimensions[selected.value.index] ?? null) : null,
);
const curFilter = computed(() =>
  selected.value.kind === "filter" ? (config.value.Filters[selected.value.index] ?? null) : null,
);

// Compact one-line summaries shown on each sidebar chip.
function measureSummary(m: Measure): string {
  const op = opLabelKeys[m.Op] ? t(opLabelKeys[m.Op]) : m.Op;
  if (!opNeedsSource(m.Op) || !m.Source) return op;
  return `${op} · ${sourceByKey.value[srcKey(m.Source)]?.label ?? m.Source.Key}`;
}
function dimensionSummary(d: Dimension): string {
  const lbl = sourceByKey.value[srcKey(d.Source)]?.label ?? d.Source.Key;
  return d.Top ? `${lbl} · top ${d.Top}` : lbl;
}
function filterSummary(f: Filter): string {
  const lbl = sourceByKey.value[srcKey(f.Source)]?.label ?? f.Source.Key;
  return `${lbl} ${f.Op} ${f.Value || "…"}`;
}
function percentSummary(): string {
  const v = config.value.Percent || "distribution";
  return pctBaseLabelKeys[v] ? t(pctBaseLabelKeys[v]) : v;
}
function scaleSummary(): string {
  if (!config.value.Scale) return t("workspace.templates.stat_builder.scale.none");
  const s = (props.scalings ?? []).find((x) => x.name === config.value.Scale);
  return s?.label || config.value.Scale;
}

// Add-then-select wrappers, so a new block opens in the editor.
function addMeasureSel() {
  addMeasure();
  selectBlock("measure", config.value.Measures.length - 1);
}
function addDimensionSel() {
  const n = config.value.Dimensions.length;
  addDimension();
  if (config.value.Dimensions.length > n) selectBlock("dimension", n);
}
function addFilterSel() {
  const n = config.value.Filters.length;
  addFilter();
  if (config.value.Filters.length > n) selectBlock("filter", n);
}
function removeMeasureSel(i: number) {
  removeMeasure(i);
  selectSafe();
}
function removeDimensionSel(i: number) {
  removeDimension(i);
  selectSafe();
}
function removeFilterSel(i: number) {
  removeFilter(i);
  selectSafe();
}

// ── DSL compile ─────────────────────────────────────────────────────
// recompile() turns the block sentence into the canonical DSL; StatLivePreview
// (a secondary component) takes that DSL and renders the live chart.
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
  return { Measures: [{ Op: MeasureOp.OpCount, Source: null, Arg: null }], Dimensions: [], Filters: [], Percent: "", Scale: "" };
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return;
    parseWarn.value = false;
    compileError.value = "";
    dslPreview.value = "";
    if (measureOps.value.length === 0) {
      const [ops, bs, fops, pbs] = await Promise.all([
        StatSvc.BuilderMeasureOps(),
        StatSvc.BuilderBins(),
        StatSvc.BuilderFilterOps(),
        StatSvc.BuilderPercentBases(),
      ]);
      measureOps.value = ops;
      bins.value = bs;
      filterOps.value = fops;
      percentBases.value = pbs;
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
            Filters: (parsed.Filters ?? []) as Filter[],
            Percent: (parsed.Percent ?? "") as string,
            Scale: (parsed.Scale ?? "") as string,
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
    selected.value = { kind: "measure", index: 0 };
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
    width="860px"
    scroll
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

      <!-- BLOCKS: left = the statistic's outline, right = the selected block -->
      <div class="stat-builder-split">
        <aside class="stat-builder-blocklist">
          <div class="stat-block-group">{{ t('workspace.templates.stat_builder.measures') }}</div>
          <draggable
            v-model="config.Measures"
            tag="div"
            class="stat-block-dndlist"
            handle=".dnd-handle"
            :animation="150"
            ghost-class="dnd-ghost"
            chosen-class="dnd-chosen"
            drag-class="dnd-drag"
            :item-key="blockKey"
          >
            <template #item="{ element: m, index: i }">
              <button
                type="button"
                :class="['stat-block-item', 'is-measure', { 'is-selected': isSelected('measure', i) }]"
                @click="selectBlock('measure', i)"
              >
                <span class="dnd-handle" aria-hidden="true">☰</span>
                <span class="stat-block-text">{{ measureSummary(m) }}</span>
              </button>
            </template>
          </draggable>
          <button type="button" class="stat-block-add" @click="addMeasureSel">
            + {{ t('workspace.templates.stat_builder.add_measure') }}
          </button>

          <div class="stat-block-group">{{ t('workspace.templates.stat_builder.dimensions') }}</div>
          <draggable
            v-model="config.Dimensions"
            tag="div"
            class="stat-block-dndlist"
            handle=".dnd-handle"
            :animation="150"
            ghost-class="dnd-ghost"
            chosen-class="dnd-chosen"
            drag-class="dnd-drag"
            :item-key="blockKey"
          >
            <template #item="{ element: d, index: i }">
              <button
                type="button"
                :class="['stat-block-item', 'is-dimension', { 'is-selected': isSelected('dimension', i) }]"
                @click="selectBlock('dimension', i)"
              >
                <span class="dnd-handle" aria-hidden="true">☰</span>
                <span class="stat-block-text">{{ dimensionSummary(d) }}</span>
              </button>
            </template>
          </draggable>
          <button type="button" class="stat-block-add" :disabled="!hasSources" @click="addDimensionSel">
            + {{ t('workspace.templates.stat_builder.add_dimension') }}
          </button>

          <div class="stat-block-group">{{ t('workspace.templates.stat_builder.filters') }}</div>
          <button
            v-for="(f, i) in config.Filters"
            :key="`bf${i}`"
            type="button"
            :class="['stat-block-item', 'is-filter', { 'is-selected': isSelected('filter', i) }]"
            @click="selectBlock('filter', i)"
          >
            <span class="stat-block-text">{{ filterSummary(f) }}</span>
          </button>
          <button type="button" class="stat-block-add" :disabled="!hasSources" @click="addFilterSel">
            + {{ t('workspace.templates.stat_builder.add_filter') }}
          </button>

          <div class="stat-block-group">{{ t('workspace.templates.stat_builder.pct.legend') }}</div>
          <button
            type="button"
            :class="['stat-block-item', 'is-percent', { 'is-selected': isSelected('percent', 0) }]"
            @click="selectBlock('percent', 0)"
          >{{ percentSummary() }}</button>

          <template v-if="hasScalings">
            <div class="stat-block-group">{{ t('workspace.templates.stat_builder.scale.legend') }}</div>
            <button
              type="button"
              :class="['stat-block-item', 'is-scale', { 'is-selected': isSelected('scale', 0) }]"
              @click="selectBlock('scale', 0)"
            >{{ scaleSummary() }}</button>
          </template>
        </aside>

        <section class="stat-builder-blockedit">
          <!-- MEASURE -->
          <template v-if="curMeasure">
            <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.measures') }}</span>
            <SelectField
              :model-value="curMeasure.Op"
              :options="measureOpOptions"
              @update:model-value="(v: string) => setMeasureOp(selected.index, v)"
            />
            <SelectField
              v-if="opNeedsSource(curMeasure.Op)"
              :model-value="curMeasure.Source ? srcKey(curMeasure.Source) : ''"
              :options="numericSourceOptions"
              @update:model-value="(v: string) => setMeasureSource(selected.index, v)"
            />
            <TextField
              v-if="opNeedsArg(curMeasure.Op)"
              type="number"
              lazy
              :min="0"
              :max="100"
              :step="1"
              :model-value="String(curMeasure.Arg ?? 90)"
              class="stat-builder-arg"
              @update:model-value="(v: string) => setMeasureArg(selected.index, v)"
            />
            <button
              type="button"
              class="tool-btn danger stat-block-remove"
              :disabled="config.Measures.length <= 1"
              @click="removeMeasureSel(selected.index)"
            >{{ t('workspace.templates.stat_builder.remove') }}</button>
          </template>

          <!-- GROUP BY -->
          <template v-else-if="curDimension">
            <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.dimensions') }}</span>
            <p class="muted small stat-builder-hint">{{ t('workspace.templates.stat_builder.dimensions_hint') }}</p>
            <SelectField
              :model-value="srcKey(curDimension.Source)"
              :options="allSourceOptions"
              @update:model-value="(v: string) => setDimensionSource(selected.index, v)"
            />
            <SelectField
              v-if="dimIsDate(curDimension)"
              :model-value="curDimension.Bin"
              :options="binOptions"
              @update:model-value="(v: string) => setDimensionBin(selected.index, v)"
            />
            <label class="stat-builder-field">
              <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.top') }}</span>
              <TextField
                type="number"
                lazy
                :min="1"
                :max="20"
                :step="1"
                :model-value="curDimension.Top ? String(curDimension.Top) : ''"
                class="stat-builder-arg"
                :title="t('workspace.templates.stat_builder.top_hint')"
                @update:model-value="(v: string) => setDimensionTop(selected.index, v)"
              />
            </label>
            <button type="button" class="tool-btn danger stat-block-remove" @click="removeDimensionSel(selected.index)">
              {{ t('workspace.templates.stat_builder.remove') }}
            </button>
          </template>

          <!-- WHERE -->
          <template v-else-if="curFilter">
            <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.filters') }}</span>
            <SelectField
              :model-value="srcKey(curFilter.Source)"
              :options="allSourceOptions"
              @update:model-value="(v: string) => setFilterSource(selected.index, v)"
            />
            <SelectField
              :model-value="curFilter.Op"
              :options="filterOpOptionsFor(curFilter)"
              @update:model-value="(v: string) => setFilterOp(selected.index, v)"
            />
            <SelectField
              v-if="filterChoicesFor(curFilter)"
              :model-value="curFilter.Value"
              :options="filterChoicesFor(curFilter)!"
              @update:model-value="(v: string) => setFilterValue(selected.index, v)"
            />
            <TextField
              v-else
              :type="filterOpIsNumeric(curFilter.Op) ? 'number' : 'text'"
              lazy
              :model-value="curFilter.Value"
              :placeholder="t('workspace.templates.stat_builder.filter_value')"
              @update:model-value="(v: string) => setFilterValue(selected.index, v)"
            />
            <button type="button" class="tool-btn danger stat-block-remove" @click="removeFilterSel(selected.index)">
              {{ t('workspace.templates.stat_builder.remove') }}
            </button>
          </template>

          <!-- PERCENTAGE BASE -->
          <template v-else-if="selected.kind === 'percent'">
            <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.pct.legend') }}</span>
            <p class="muted small stat-builder-hint">{{ t('workspace.templates.stat_builder.pct.hint') }}</p>
            <SelectField
              :model-value="config.Percent || 'distribution'"
              :options="percentBaseOptions"
              @update:model-value="setPercent"
            />
          </template>

          <!-- SCALE (weighting) -->
          <template v-else-if="selected.kind === 'scale'">
            <span class="stat-builder-field-label">{{ t('workspace.templates.stat_builder.scale.legend') }}</span>
            <p class="muted small stat-builder-hint">{{ t('workspace.templates.stat_builder.scale.hint') }}</p>
            <SelectField
              :model-value="config.Scale"
              :options="scaleOptions"
              @update:model-value="setScale"
            />
          </template>

          <p v-else class="muted small">{{ t('workspace.templates.stat_builder.select_block') }}</p>
        </section>
      </div>

      <!-- DSL readout (advanced) -->
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
