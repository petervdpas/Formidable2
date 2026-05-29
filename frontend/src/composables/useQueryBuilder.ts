// useQueryBuilder owns the query builder's state and logic so the dialog
// and its tab components share one source of truth. QueryDialog creates it
// and keeps the run/transport lifecycle; the tab components (QueryColumns,
// QueryFilters, QueryGroup, QueryOrder, QueryText) are presentational and
// read/mutate this builder. The returned object is reactive, so a tab can
// bind builder.columns / builder.distinct directly.

import { computed, reactive, ref, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  Service as QuerySvc,
  Spec,
  type SourceInfo,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/query";

// Explicit key maps (never interpolate a lookup key).
export const OP_LABEL_KEYS: Record<string, string> = {
  eq: "query.op.eq",
  ne: "query.op.ne",
  lt: "query.op.lt",
  le: "query.op.le",
  gt: "query.op.gt",
  ge: "query.op.ge",
};

export const AGG_FUNCS = ["count", "count_distinct", "sum", "avg", "min", "max"] as const;
export type AggFunc = (typeof AGG_FUNCS)[number];
export const AGG_LABEL_KEYS: Record<AggFunc, string> = {
  count: "query.agg.count",
  count_distinct: "query.agg.count_distinct",
  sum: "query.agg.sum",
  avg: "query.agg.avg",
  min: "query.agg.min",
  max: "query.agg.max",
};

export function needsSource(fn: string): boolean {
  return fn === "sum" || fn === "avg" || fn === "min" || fn === "max";
}

export interface ColumnRow {
  id: string;
  header: string;
  sourceId: string;
}
export interface FilterRow {
  id: string;
  sourceId: string;
  op: string;
  value: string;
}
export interface MeasureRow {
  id: string;
  func: AggFunc;
  sourceId: string;
  header: string;
}
export interface OrderRow {
  id: string;
  targetKey: string; // a column id, or "m:<measure id>"
  desc: boolean;
}

export function useQueryBuilder(templateFilename: Ref<string>) {
  const { t } = useI18n();

  const sources = ref<SourceInfo[]>([]);
  const ops = ref<string[]>([]);
  const columns = ref<ColumnRow[]>([]);
  const filters = ref<FilterRow[]>([]);
  const measures = ref<MeasureRow[]>([]);
  const orders = ref<OrderRow[]>([]);
  const groupDims = ref<string[]>([]);
  const distinct = ref(false);
  const limit = ref(0);
  const sqlText = ref("");
  let seq = 0;

  const sourceById = computed<Record<string, SourceInfo>>(() => {
    const m: Record<string, SourceInfo> = {};
    for (const s of sources.value) m[s.id] = s;
    return m;
  });
  const aggregatableSources = computed(() => sources.value.filter((s) => s.aggregatable));

  const sourceLabel = (id: string) => sourceById.value[id]?.label ?? "";
  const colLabel = (c: ColumnRow) => c.header || sourceLabel(c.sourceId);

  const validColumns = computed(() => columns.value.filter((c) => c.sourceId && sourceById.value[c.sourceId]));
  const grouping = computed(() => validColumns.value.some((c) => groupDims.value.includes(c.id)));
  const canRun = computed(() => validColumns.value.length > 0);

  // In group mode with no measures added, the backend still wants a count;
  // surface one so the result has a measure column.
  const effectiveMeasures = computed<MeasureRow[]>(() =>
    grouping.value && measures.value.length === 0
      ? [{ id: "auto-count", func: "count", sourceId: "", header: "" }]
      : measures.value,
  );

  // Ordering targets, mode-aware: in group mode the output columns are the
  // group dimensions then the measures; otherwise the projected columns.
  // The index of a target here equals its index in the result.
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

  function reset() {
    columns.value = [];
    filters.value = [];
    measures.value = [];
    orders.value = [];
    groupDims.value = [];
    distinct.value = false;
    limit.value = 0;
    sqlText.value = "";
    seq = 0;
  }

  // load fetches the backend-owned source list (and operators once), then
  // resets the builder. Throws on a sources error so the dialog can surface
  // it; the operator list degrades to the local key set.
  async function load() {
    reset();
    sources.value = await QuerySvc.Sources(templateFilename.value);
    if (ops.value.length === 0) {
      try {
        ops.value = await QuerySvc.FilterOps();
      } catch {
        ops.value = Object.keys(OP_LABEL_KEYS);
      }
    }
  }

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
      template: templateFilename.value,
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
    sqlText.value = await QuerySvc.Explain(buildSpec());
  }

  return reactive({
    sources,
    ops,
    columns,
    filters,
    measures,
    orders,
    groupDims,
    distinct,
    limit,
    sqlText,
    sourceById,
    aggregatableSources,
    validColumns,
    grouping,
    canRun,
    effectiveMeasures,
    orderTargets,
    sourceLabel,
    colLabel,
    load,
    reset,
    addColumn,
    removeColumn,
    addFilter,
    removeFilter,
    addMeasure,
    removeMeasure,
    addOrder,
    removeOrder,
    buildSpec,
    refreshSql,
  });
}

export type QueryBuilder = ReturnType<typeof useQueryBuilder>;
