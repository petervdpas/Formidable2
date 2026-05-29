<script setup lang="ts">
/**
 * QueryDialog - the studio's read-only query surface (FDRM). Triggered
 * from the StorageWorkspace "Data" menu. Pick columns and filters over the
 * template's indexed values (use_in_statistics fields + facets), run, and
 * read the result table; the result can be exported through the existing
 * CSV writer. Owns no SQL - it builds a query.Spec and calls Query.Run.
 */
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SwitchField } from "./fields";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import { deriveQueryableSources, type QueryableSource } from "../composables/useQueryableSources";
import {
  Service as QuerySvc,
  Spec,
  type Result,
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

// Explicit op-key -> i18n-key map (never interpolate the lookup key).
const OP_LABEL_KEYS: Record<string, string> = {
  eq: "query.op.eq",
  ne: "query.op.ne",
  lt: "query.op.lt",
  le: "query.op.le",
  gt: "query.op.gt",
  ge: "query.op.ge",
};

type SortDir = "none" | "asc" | "desc";
interface ColumnRow {
  id: string;
  header: string;
  sourceId: string;
  group: boolean;
  sort: SortDir;
}
interface FilterRow {
  id: string;
  sourceId: string;
  op: string;
  value: string;
}

const sources = ref<QueryableSource[]>([]);
const sourceById = computed<Record<string, QueryableSource>>(() => {
  const m: Record<string, QueryableSource> = {};
  for (const s of sources.value) m[s.id] = s;
  return m;
});
const ops = ref<string[]>([]);
const columns = ref<ColumnRow[]>([]);
const filters = ref<FilterRow[]>([]);
const distinct = ref(false);
const count = ref(true);
const limit = ref(0);
const result = ref<Result | null>(null);
const running = ref(false);
const errorMsg = ref("");
let seq = 0;

const grouping = computed(() => columns.value.some((c) => c.group && c.sourceId));
// Two fanning (table / list / tags) columns can't be row-aligned from the
// index, so at most one may be projected at a time.
const multiCount = computed(
  () => columns.value.filter((c) => sourceById.value[c.sourceId]?.multi).length,
);
const tooManyMulti = computed(() => multiCount.value > 1);
const canRun = computed(() => columns.value.some((c) => c.sourceId) && !tooManyMulti.value);

watch(
  () => props.open,
  async (isOpen) => {
    if (!isOpen) return;
    sources.value = deriveQueryableSources(props.template);
    distinct.value = false;
    count.value = true;
    limit.value = 0;
    filters.value = [];
    result.value = null;
    errorMsg.value = "";
    running.value = false;
    seq = 0;
    // Default to every queryable source as a column, like CSV export, so
    // the user sees data on the first Run and trims down from there. Keep
    // the default valid: include at most one fanning (table/list) column.
    {
      const cols: ColumnRow[] = [];
      let multiUsed = false;
      for (const s of sources.value) {
        if (s.multi) {
          if (multiUsed) continue;
          multiUsed = true;
        }
        cols.push({ id: `col-${seq++}`, header: s.label, sourceId: s.id, group: false, sort: "none" });
      }
      columns.value = cols;
    }
    if (ops.value.length === 0) {
      try {
        ops.value = await QuerySvc.FilterOps();
      } catch {
        ops.value = Object.keys(OP_LABEL_KEYS);
      }
    }
  },
);

function addColumn() {
  columns.value.push({ id: `col-${seq++}`, header: "", sourceId: sources.value[0]?.id ?? "", group: false, sort: "none" });
}
function removeColumn(i: number) {
  columns.value.splice(i, 1);
}
function addFilter() {
  filters.value.push({ id: `flt-${seq++}`, sourceId: sources.value[0]?.id ?? "", op: ops.value[0] ?? "eq", value: "" });
}
function removeFilter(i: number) {
  filters.value.splice(i, 1);
}

function buildSpec(): Spec {
  // One filtered list so column indices in groupBy / orderBy line up with
  // the columns actually sent.
  const valid = columns.value.filter((c) => c.sourceId && sourceById.value[c.sourceId]);
  const cols = valid.map((c) => ({
    header: c.header || sourceById.value[c.sourceId].label,
    source: sourceById.value[c.sourceId].source,
  }));
  const groupBy: number[] = [];
  const orderBy: { column: number; desc: boolean; numeric: boolean }[] = [];
  valid.forEach((c, i) => {
    if (c.group) groupBy.push(i);
    if (c.sort !== "none") {
      orderBy.push({ column: i, desc: c.sort === "desc", numeric: !!sourceById.value[c.sourceId].numeric });
    }
  });
  const flts = filters.value
    .filter((f) => f.sourceId && sourceById.value[f.sourceId])
    .map((f) => ({ source: sourceById.value[f.sourceId].source, op: f.op, value: f.value }));
  return Spec.createFrom({
    template: props.templateFilename,
    columns: cols,
    filters: flts,
    distinct: groupBy.length === 0 && distinct.value,
    groupBy,
    count: groupBy.length > 0 && count.value,
    orderBy,
    limit: limit.value > 0 ? limit.value : 0,
  });
}

async function run() {
  if (!canRun.value) return;
  running.value = true;
  errorMsg.value = "";
  try {
    result.value = await QuerySvc.Run(buildSpec());
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

    <section class="query-section">
      <header class="query-section-head">
        <h4>{{ t('query.columns') }}</h4>
        <button type="button" class="tool-btn" :disabled="sources.length === 0" @click="addColumn">
          {{ t('query.add_column') }}
        </button>
      </header>
      <table class="query-table">
        <thead>
          <tr>
            <th>{{ t('query.column.source') }}</th>
            <th>{{ t('query.column.header') }}</th>
            <th class="query-th-narrow">{{ t('query.column.group') }}</th>
            <th class="query-th-narrow">{{ t('query.column.sort') }}</th>
            <th class="query-th-narrow"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(c, i) in columns" :key="c.id">
            <td>
              <select v-model="c.sourceId" :class="{ 'query-multi': sourceById[c.sourceId]?.multi }">
                <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.label }}</option>
              </select>
            </td>
            <td><input v-model="c.header" type="text" :placeholder="sourceById[c.sourceId]?.label" /></td>
            <td class="query-td-center"><input v-model="c.group" type="checkbox" /></td>
            <td>
              <select v-model="c.sort">
                <option value="none">{{ t('query.sort.none') }}</option>
                <option value="asc">{{ t('query.sort.asc') }}</option>
                <option value="desc">{{ t('query.sort.desc') }}</option>
              </select>
            </td>
            <td class="query-td-center">
              <button type="button" class="tool-btn danger" @click="removeColumn(i)">×</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="tooManyMulti" class="form-error">{{ t('query.too_many_multi') }}</p>
    </section>

    <section class="query-section">
      <header class="query-section-head">
        <h4>{{ t('query.filters') }}</h4>
        <button type="button" class="tool-btn" :disabled="sources.length === 0" @click="addFilter">
          {{ t('query.add_filter') }}
        </button>
      </header>
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
    </section>

    <section class="query-options">
      <SwitchField v-if="!grouping" v-model="distinct" :on-label="t('query.distinct')" />
      <SwitchField v-if="grouping" v-model="count" :on-label="t('query.count')" />
      <label class="query-limit">
        {{ t('query.limit') }}
        <input v-model.number="limit" type="number" min="0" />
      </label>
    </section>

    <section v-if="result" class="query-results">
      <p class="query-result-count">{{ t('query.result_count', { count: result.count, total: result.total }) }}</p>
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
