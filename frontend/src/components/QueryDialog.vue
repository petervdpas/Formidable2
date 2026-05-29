<script setup lang="ts">
/**
 * QueryDialog - the studio's read-only query surface (FDRM). Triggered
 * from the StorageWorkspace "Data" menu. The backend owns everything
 * structural: it lists the queryable sources (fields, table columns,
 * facets) and their capabilities, renders the SQL preview, and runs the
 * query over an in-memory matrix it prepares from the form data. This
 * dialog only assembles a query.Spec and displays the result.
 *
 * Four tabs keep the concerns apart: Columns (drag to reorder), Filters,
 * Group (group-by dimensions + aggregate measures), Order, and a read-only
 * SQL preview rendered by the backend.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import Modal from "./Modal.vue";
import Tabs from "./Tabs.vue";
import { SwitchField } from "./fields";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import {
  Service as QuerySvc,
  Spec,
  type Result,
  type SourceInfo,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/query";
import { Service as CsvSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/csv";
import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  open: boolean;
  templateFilename: string;
  template: Template | null;
}>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();
const { chooseSaveFile } = useDialog();
const toast = useToast();

// Explicit key maps (never interpolate a lookup key).
const OP_LABEL_KEYS: Record<string, string> = {
  eq: "query.op.eq",
  ne: "query.op.ne",
  lt: "query.op.lt",
  le: "query.op.le",
  gt: "query.op.gt",
  ge: "query.op.ge",
};
const AGG_FUNCS = ["count", "count_distinct", "sum", "avg", "min", "max"] as const;
type AggFunc = (typeof AGG_FUNCS)[number];
const AGG_LABEL_KEYS: Record<AggFunc, string> = {
  count: "query.agg.count",
  count_distinct: "query.agg.count_distinct",
  sum: "query.agg.sum",
  avg: "query.agg.avg",
  min: "query.agg.min",
  max: "query.agg.max",
};
function needsSource(fn: string): boolean {
  return fn === "sum" || fn === "avg" || fn === "min" || fn === "max";
}

interface ColumnRow {
  id: string;
  header: string;
  sourceId: string;
}
interface FilterRow {
  id: string;
  sourceId: string;
  op: string;
  value: string;
}
interface MeasureRow {
  id: string;
  func: AggFunc;
  sourceId: string;
  header: string;
}
interface OrderRow {
  id: string;
  targetKey: string; // a column id, or "m:<measure id>"
  desc: boolean;
}

const sources = ref<SourceInfo[]>([]);
const sourceById = computed<Record<string, SourceInfo>>(() => {
  const m: Record<string, SourceInfo> = {};
  for (const s of sources.value) m[s.id] = s;
  return m;
});
const aggregatableSources = computed(() => sources.value.filter((s) => s.aggregatable));

const ops = ref<string[]>([]);
const columns = ref<ColumnRow[]>([]);
const filters = ref<FilterRow[]>([]);
const measures = ref<MeasureRow[]>([]);
const orders = ref<OrderRow[]>([]);
const groupDims = ref<string[]>([]); // column ids used as group dimensions
const distinct = ref(false);
const limit = ref(0);
const result = ref<Result | null>(null);
const running = ref(false);
const errorMsg = ref("");
const activeTab = ref("columns");
const sqlText = ref("");
let seq = 0;

const sourceLabel = (id: string) => sourceById.value[id]?.label ?? "";
const colLabel = (c: ColumnRow) => c.header || sourceLabel(c.sourceId);

const validColumns = computed(() => columns.value.filter((c) => c.sourceId && sourceById.value[c.sourceId]));
const grouping = computed(() => validColumns.value.some((c) => groupDims.value.includes(c.id)));
const canRun = computed(() => validColumns.value.length > 0);

// Ordering targets, mode-aware: in group mode the output columns are the
// group dimensions then the measures; otherwise the projected columns.
// The index of a target here equals its index in the result, so an
// OrderRow maps straight to Sort.column.
const orderTargets = computed(() => {
  if (grouping.value) {
    const dims = validColumns.value
      .filter((c) => groupDims.value.includes(c.id))
      .map((c) => ({ key: c.id, label: colLabel(c), numeric: !!sourceById.value[c.sourceId]?.numeric }));
    const meas = effectiveMeasures.value.map((m) => ({
      key: `m:${m.id}`,
      label: m.header || t(AGG_LABEL_KEYS[m.func]),
      numeric: true,
    }));
    return [...dims, ...meas];
  }
  return validColumns.value.map((c) => ({ key: c.id, label: colLabel(c), numeric: !!sourceById.value[c.sourceId]?.numeric }));
});

// In group mode with no measures added, the backend still wants a count;
// surface one so the result has a measure column.
const effectiveMeasures = computed<MeasureRow[]>(() =>
  grouping.value && measures.value.length === 0
    ? [{ id: "auto-count", func: "count", sourceId: "", header: "" }]
    : measures.value,
);

const tabItems = computed(() => [
  { id: "columns", label: t("query.columns") },
  { id: "filters", label: t("query.filters") },
  { id: "group", label: t("query.group") },
  { id: "order", label: t("query.order") },
  { id: "sql", label: t("query.sql") },
]);

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    distinct.value = false;
    limit.value = 0;
    filters.value = [];
    measures.value = [];
    orders.value = [];
    groupDims.value = [];
    result.value = null;
    errorMsg.value = "";
    running.value = false;
    sqlText.value = "";
    activeTab.value = "columns";
    seq = 0;
    columns.value = [];
    try {
      sources.value = await QuerySvc.Sources(props.templateFilename);
    } catch (e) {
      sources.value = [];
      errorMsg.value = backendErrMessage(e);
    }
    // Every tab starts empty: the user builds the query deliberately rather
    // than trimming a prefilled set.
    columns.value = [];
    if (ops.value.length === 0) {
      try {
        ops.value = await QuerySvc.FilterOps();
      } catch {
        ops.value = Object.keys(OP_LABEL_KEYS);
      }
    }
  },
);

// Refresh the backend-rendered SQL whenever the SQL tab is opened.
watch(activeTab, (tab) => {
  if (tab === "sql") void refreshSql();
});

function addColumn() {
  columns.value.push({ id: `col-${seq++}`, header: "", sourceId: sources.value[0]?.id ?? "" });
}
function removeColumn(i: number) {
  const removed = columns.value[i];
  columns.value.splice(i, 1);
  if (removed) {
    groupDims.value = groupDims.value.filter((id) => id !== removed.id);
    orders.value = orders.value.filter((o) => o.targetKey !== removed.id);
  }
}
function addFilter() {
  filters.value.push({ id: `flt-${seq++}`, sourceId: sources.value[0]?.id ?? "", op: ops.value[0] ?? "eq", value: "" });
}
function removeFilter(i: number) {
  filters.value.splice(i, 1);
}
function addMeasure() {
  measures.value.push({ id: `mea-${seq++}`, func: "count", sourceId: aggregatableSources.value[0]?.id ?? "", header: "" });
}
function removeMeasure(i: number) {
  const removed = measures.value[i];
  measures.value.splice(i, 1);
  if (removed) orders.value = orders.value.filter((o) => o.targetKey !== `m:${removed.id}`);
}
function addOrder() {
  orders.value.push({ id: `ord-${seq++}`, targetKey: orderTargets.value[0]?.key ?? "", desc: false });
}
function removeOrder(i: number) {
  orders.value.splice(i, 1);
}

function buildSpec(): Spec {
  const valid = validColumns.value;
  const cols = valid.map((c) => ({ header: c.header || sourceLabel(c.sourceId), source: sourceById.value[c.sourceId].source }));

  const groupBy: number[] = [];
  valid.forEach((c, i) => {
    if (groupDims.value.includes(c.id)) groupBy.push(i);
  });
  const isGroup = groupBy.length > 0;

  const measureSpec = isGroup
    ? effectiveMeasures.value.map((m) => ({
        func: m.func,
        source: needsSource(m.func) && sourceById.value[m.sourceId] ? sourceById.value[m.sourceId].source : { kind: "field", key: "" },
        header: m.header || t(AGG_LABEL_KEYS[m.func]),
      }))
    : [];

  const targets = orderTargets.value;
  const orderBy = orders.value
    .map((o) => {
      const idx = targets.findIndex((tg) => tg.key === o.targetKey);
      return idx < 0 ? null : { column: idx, desc: o.desc, numeric: targets[idx].numeric };
    })
    .filter((s): s is { column: number; desc: boolean; numeric: boolean } => s !== null);

  const flts = filters.value
    .filter((f) => f.sourceId && sourceById.value[f.sourceId])
    .map((f) => ({ source: sourceById.value[f.sourceId].source, op: f.op, value: f.value }));

  return Spec.createFrom({
    template: props.templateFilename,
    columns: cols,
    filters: flts,
    distinct: !isGroup && distinct.value,
    groupBy,
    measures: measureSpec,
    orderBy,
    limit: limit.value > 0 ? limit.value : 0,
  });
}

async function refreshSql() {
  if (!canRun.value) {
    sqlText.value = "";
    return;
  }
  try {
    sqlText.value = await QuerySvc.Explain(buildSpec());
  } catch (e) {
    sqlText.value = "";
    errorMsg.value = backendErrMessage(e);
  }
}

async function run() {
  if (!canRun.value) return;
  running.value = true;
  errorMsg.value = "";
  try {
    result.value = await QuerySvc.Run(buildSpec());
    if (activeTab.value === "sql") void refreshSql();
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    result.value = null;
    toast.error("query.failed");
  } finally {
    running.value = false;
  }
}

async function exportCsv() {
  const res = result.value;
  if (!res || res.rows.length === 0) return;
  try {
    const stem = props.templateFilename.replace(/\.yaml$/, "");
    const path = await chooseSaveFile(`${stem}-query.csv`, [{ displayName: "CSV", pattern: "*.csv" }]);
    if (!path) return;
    const rows: string[][] = [res.columns, ...res.rows.map((r) => r.map((c) => c.text))];
    const write = await CsvSvc.Write(path, rows, ",");
    if (!write.success) {
      toast.error("query.failed");
      return;
    }
    toast.success("query.exported", [res.rows.length]);
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    toast.error("query.failed");
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('query.title')"
    width="860px"
    scroll
    maximizable
    @close="emit('close')"
  >
    <template #head>
      <div class="query-target">
        <span class="query-target-label">{{ t('query.template') }}:</span>
        <code class="query-target-value">{{ template?.name || templateFilename }}</code>
      </div>
      <p v-if="sources.length === 0" class="form-description">{{ t('query.empty_sources') }}</p>
      <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>
    </template>

    <Tabs v-model="activeTab" :items="tabItems">
      <template #columns>
        <div class="query-section-head">
          <span class="form-description">{{ t('query.columns_hint') }}</span>
          <button type="button" class="tool-btn" :disabled="sources.length === 0" @click="addColumn">
            {{ t('query.add_column') }}
          </button>
        </div>
        <draggable
          :list="columns"
          tag="div"
          class="query-col-list"
          handle=".dnd-handle"
          :animation="150"
          ghost-class="dnd-ghost"
          chosen-class="dnd-chosen"
          drag-class="dnd-drag"
          item-key="id"
        >
          <template #item="{ element: c, index: i }">
            <div class="query-col-row">
              <span class="dnd-handle" aria-hidden="true">☰</span>
              <select v-model="c.sourceId" class="query-col-source" :class="{ 'query-multi': sourceById[c.sourceId]?.fans }">
                <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.label }}</option>
              </select>
              <input v-model="c.header" type="text" class="query-col-header" :placeholder="sourceById[c.sourceId]?.label" />
              <button type="button" class="tool-btn danger" @click="removeColumn(i)">×</button>
            </div>
          </template>
        </draggable>
        <p v-if="!columns.length" class="form-description">{{ t('query.no_columns') }}</p>
      </template>

      <template #filters>
        <div class="query-section-head">
          <span class="form-description">{{ t('query.filters_hint') }}</span>
          <button type="button" class="tool-btn" :disabled="sources.length === 0" @click="addFilter">
            {{ t('query.add_filter') }}
          </button>
        </div>
        <table v-if="filters.length" class="query-table">
          <thead>
            <tr>
              <th>{{ t('query.column.source') }}</th>
              <th class="query-th-narrow">{{ t('query.filter.op') }}</th>
              <th>{{ t('query.filter.value') }}</th>
              <th class="query-th-narrow"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(f, i) in filters" :key="f.id">
              <td>
                <select v-model="f.sourceId">
                  <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.label }}</option>
                </select>
              </td>
              <td>
                <select v-model="f.op">
                  <option v-for="op in ops" :key="op" :value="op">{{ t(OP_LABEL_KEYS[op] || op) }}</option>
                </select>
              </td>
              <td>
                <select v-if="sourceById[f.sourceId]?.choices" v-model="f.value">
                  <option value=""></option>
                  <option v-for="ch in sourceById[f.sourceId]!.choices" :key="ch.value" :value="ch.value">
                    {{ ch.label }}
                  </option>
                </select>
                <input v-else v-model="f.value" type="text" />
              </td>
              <td class="query-td-center">
                <button type="button" class="tool-btn danger" @click="removeFilter(i)">×</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-else class="form-description">{{ t('query.no_filters') }}</p>
      </template>

      <template #group>
        <div class="query-section-head">
          <span class="form-description">{{ t('query.group_hint') }}</span>
        </div>
        <div v-if="validColumns.length" class="query-col-list">
          <label v-for="c in validColumns" :key="c.id" class="query-col-group-row">
            <input type="checkbox" :value="c.id" v-model="groupDims" />
            {{ colLabel(c) }}
          </label>
        </div>
        <p v-else class="form-description">{{ t('query.no_columns') }}</p>

        <div class="query-section-head">
          <h4>{{ t('query.measures') }}</h4>
          <button type="button" class="tool-btn" :disabled="!grouping" @click="addMeasure">
            {{ t('query.add_measure') }}
          </button>
        </div>
        <div v-if="grouping && measures.length" class="query-col-list">
          <div v-for="(ms, i) in measures" :key="ms.id" class="query-col-row">
            <select v-model="ms.func">
              <option v-for="fn in AGG_FUNCS" :key="fn" :value="fn">{{ t(AGG_LABEL_KEYS[fn]) }}</option>
            </select>
            <select v-if="needsSource(ms.func)" v-model="ms.sourceId" class="query-col-source">
              <option v-for="s in aggregatableSources" :key="s.id" :value="s.id">{{ s.label }}</option>
            </select>
            <input v-model="ms.header" type="text" class="query-col-header" :placeholder="t(AGG_LABEL_KEYS[ms.func])" />
            <button type="button" class="tool-btn danger" @click="removeMeasure(i)">×</button>
          </div>
        </div>
        <p v-else class="form-description">{{ t('query.no_group') }}</p>
      </template>

      <template #order>
        <div class="query-section-head">
          <span class="form-description">{{ t('query.order_hint') }}</span>
          <button type="button" class="tool-btn" :disabled="orderTargets.length === 0" @click="addOrder">
            {{ t('query.add_order') }}
          </button>
        </div>
        <draggable
          v-if="orders.length"
          :list="orders"
          tag="div"
          class="query-col-list"
          handle=".dnd-handle"
          :animation="150"
          ghost-class="dnd-ghost"
          chosen-class="dnd-chosen"
          drag-class="dnd-drag"
          item-key="id"
        >
          <template #item="{ element: o, index: i }">
            <div class="query-col-row">
              <span class="dnd-handle" aria-hidden="true">☰</span>
              <select v-model="o.targetKey" class="query-col-source">
                <option v-for="tg in orderTargets" :key="tg.key" :value="tg.key">{{ tg.label }}</option>
              </select>
              <select v-model="o.desc">
                <option :value="false">{{ t('query.sort.asc') }}</option>
                <option :value="true">{{ t('query.sort.desc') }}</option>
              </select>
              <button type="button" class="tool-btn danger" @click="removeOrder(i)">×</button>
            </div>
          </template>
        </draggable>
        <p v-else class="form-description">{{ t('query.no_order') }}</p>
      </template>

      <template #sql>
        <span class="form-description">{{ t('query.sql_hint') }}</span>
        <pre v-if="sqlText" class="query-sql"><code>{{ sqlText }}</code></pre>
        <p v-else class="form-description">{{ t('query.no_columns') }}</p>
      </template>
    </Tabs>

    <section class="query-options">
      <SwitchField v-if="!grouping" v-model="distinct" :on-label="t('query.distinct')" />
      <label class="query-limit">
        {{ t('query.limit') }}
        <input v-model.number="limit" type="number" min="0" />
      </label>
    </section>

    <section v-if="result" class="query-results">
      <p class="query-result-count">{{ t('query.result_count', { count: result.count, total: result.total }) }}</p>
      <div v-if="result.anomalies && result.anomalies.length" class="form-error query-anomalies">
        <strong>{{ t('query.anomalies', { count: result.anomalies.length }) }}</strong>
        <ul>
          <li v-for="(a, ai) in result.anomalies.slice(0, 8)" :key="ai">
            {{ t('query.anomaly_item', { value: a.value, column: a.column, expected: a.expected }) }}
          </li>
        </ul>
      </div>
      <div v-if="result.rows.length === 0" class="form-description">{{ t('query.no_results') }}</div>
      <table v-else class="query-table query-result-table">
        <thead>
          <tr>
            <th v-for="(h, i) in result.columns" :key="i">{{ h }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, ri) in result.rows" :key="ri">
            <td v-for="(cell, ci) in row" :key="ci">{{ cell.text }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <template #footer>
      <button
        type="button"
        class="tool-btn"
        :disabled="!result || result.rows.length === 0"
        @click="exportCsv"
      >
        {{ t('query.export') }}
      </button>
      <button class="tool-btn" type="button" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="tool-btn primary" type="button" :disabled="!canRun || running" @click="run">
        {{ running ? t('query.running') : t('query.run') }}
      </button>
    </template>
  </Modal>
</template>
