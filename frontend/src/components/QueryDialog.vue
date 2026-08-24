<script setup lang="ts">
/**
 * QueryDialog - the studio's read-only query surface (FDRM). Triggered
 * from the StorageWorkspace "Data" menu. The backend owns everything
 * structural: it lists the queryable sources, renders the SQL preview, and
 * runs the query over an in-memory matrix prepared from the form data.
 *
 * This component is the shell: it owns the builder state (useQueryBuilder)
 * and the run/export lifecycle, and composes the per-tab components
 * (QueryColumns / QueryFilters / QueryGroup / QueryOrder / QueryText) plus
 * the result view (QueryResult). Each of those is presentational over the
 * shared builder.
 */
import { computed, ref, toRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import Tabs from "./Tabs.vue";
import QueryColumns from "./QueryColumns.vue";
import QueryFilters from "./QueryFilters.vue";
import QueryGroup from "./QueryGroup.vue";
import QueryOrder from "./QueryOrder.vue";
import QueryText from "./QueryText.vue";
import QueryResult from "./QueryResult.vue";
import QuerySavedBar from "./QuerySavedBar.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import { SwitchField } from "./fields";
import { useDialog } from "../composables/useDialog";
import { useToast } from "../composables/useToast";
import { backendErrMessage } from "../utils/backendError";
import { useQueryBuilder } from "../composables/useQueryBuilder";
import { useSavedQueries } from "../composables/useSavedQueries";
import { Service as QuerySvc, type Result } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/query";
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

const builder = useQueryBuilder(toRef(props, "templateFilename"));
const saved = useSavedQueries(toRef(props, "templateFilename"));
const confirmDeleteKey = ref("");

const result = ref<Result | null>(null);
const running = ref(false);
const errorMsg = ref("");
const activeTab = ref("columns");
const exportDelimiter = ref(",");
const exportQuoteAll = ref(true);
const exportFormat = ref<"csv" | "xlsx">("csv");
const isExcelOut = computed(() => exportFormat.value === "xlsx");

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
    result.value = null;
    errorMsg.value = "";
    running.value = false;
    activeTab.value = "columns";
    exportDelimiter.value = ",";
    exportQuoteAll.value = true;
    exportFormat.value = "csv";
    confirmDeleteKey.value = "";
    saved.clear();
    try {
      await builder.load();
    } catch (e) {
      errorMsg.value = backendErrMessage(e);
    }
    try {
      await saved.refresh();
    } catch (e) {
      errorMsg.value = backendErrMessage(e);
    }
  },
);

// Refresh the backend-rendered SQL whenever the SQL tab is opened.
watch(activeTab, (tab) => {
  if (tab === "sql") void refreshSql();
});

async function refreshSql() {
  try {
    await builder.refreshSql();
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
  }
}

async function run() {
  if (!builder.canRun) return;
  running.value = true;
  errorMsg.value = "";
  try {
    result.value = await QuerySvc.Run(builder.buildSpec());
    if (activeTab.value === "sql") void refreshSql();
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    result.value = null;
    toast.error("query.failed");
  } finally {
    running.value = false;
  }
}

// Opening a saved query replaces the builder wholesale, so the stale result
// of the previous one goes with it.
async function openSaved(key: string) {
  errorMsg.value = "";
  try {
    const spec = await saved.open(key);
    builder.loadSpec(spec);
    result.value = null;
    if (activeTab.value === "sql") void refreshSql();
    toast.success("query.saved.opened", [saved.name]);
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    toast.error("query.saved.open_failed");
  }
}

async function saveCurrent() {
  if (!builder.canRun) return;
  errorMsg.value = "";
  try {
    const stored = await saved.save(builder.buildSpec());
    toast.success("query.saved.stored", [stored.name]);
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    toast.error("query.saved.save_failed");
  }
}

async function confirmDelete() {
  const key = confirmDeleteKey.value;
  confirmDeleteKey.value = "";
  if (!key) return;
  errorMsg.value = "";
  try {
    await saved.remove(key);
    toast.success("query.saved.deleted");
  } catch (e) {
    errorMsg.value = backendErrMessage(e);
    toast.error("query.saved.delete_failed");
  }
}

async function exportData() {
  const res = result.value;
  if (!res || res.rows.length === 0) return;
  try {
    const stem = props.templateFilename.replace(/\.yaml$/, "");
    const ext = isExcelOut.value ? "xlsx" : "csv";
    const path = await chooseSaveFile(`${stem}-query.${ext}`, [
      isExcelOut.value
        ? { displayName: "Excel", pattern: "*.xlsx" }
        : { displayName: "CSV", pattern: "*.csv" },
    ]);
    if (!path) return;
    const rows: string[][] = [res.columns, ...res.rows.map((r) => r.map((c) => c.text))];
    const write = isExcelOut.value
      ? await CsvSvc.WriteExcel(path, rows, stem)
      : await CsvSvc.Write(path, rows, exportDelimiter.value, exportQuoteAll.value);
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
      <QuerySavedBar
        :saved="saved"
        :can-save="builder.canRun"
        @open="openSaved"
        @save="saveCurrent"
        @delete="(key: string) => (confirmDeleteKey = key)"
      />
      <p v-if="builder.sources.length === 0" class="form-description">{{ t('query.empty_sources') }}</p>
      <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>
    </template>

    <div class="query-tabs">
      <Tabs v-model="activeTab" :items="tabItems">
        <template #columns><QueryColumns :builder="builder" /></template>
        <template #filters><QueryFilters :builder="builder" /></template>
        <template #group><QueryGroup :builder="builder" /></template>
        <template #order><QueryOrder :builder="builder" /></template>
        <template #sql><QueryText :builder="builder" /></template>
      </Tabs>
    </div>

    <section class="query-options">
      <SwitchField v-if="!builder.grouping" v-model="builder.distinct" :on-label="t('query.distinct')" />
      <label class="query-limit">
        {{ t('query.limit') }}
        <input v-model.number="builder.limit" type="number" min="0" />
      </label>
    </section>

    <QueryResult :result="result" />

    <template #footer>
      <div class="query-export-opts">
        <label class="query-export-field">
          {{ t('csv.export.format') }}
          <select v-model="exportFormat">
            <option value="csv">{{ t('csv.export.format.csv') }}</option>
            <option value="xlsx">{{ t('csv.export.format.xlsx') }}</option>
          </select>
        </label>
        <label v-if="!isExcelOut" class="query-export-field">
          {{ t('csv.delimiter') }}
          <select v-model="exportDelimiter">
            <option value=",">{{ t('csv.delimiter.comma') }}</option>
            <option value=";">{{ t('csv.delimiter.semicolon') }}</option>
            <option value="	">{{ t('csv.delimiter.tab') }}</option>
            <option value="|">{{ t('csv.delimiter.pipe') }}</option>
          </select>
        </label>
        <SwitchField v-if="!isExcelOut" v-model="exportQuoteAll" :on-label="t('query.quote')" />
      </div>
      <button
        type="button"
        class="tool-btn"
        :disabled="!result || result.rows.length === 0"
        @click="exportData"
      >
        {{ t('query.export') }}
      </button>
      <button class="tool-btn" type="button" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="tool-btn primary" type="button" :disabled="!builder.canRun || running" @click="run">
        {{ running ? t('query.running') : t('query.run') }}
      </button>
    </template>
  </Modal>

  <ConfirmDialog
    :open="confirmDeleteKey !== ''"
    elevated
    variant="danger"
    :title="t('query.saved.delete_title')"
    :message="t('query.saved.delete_confirm', [saved.selected?.name ?? ''])"
    :confirm-label="t('query.saved.delete')"
    :cancel-label="t('common.cancel')"
    @confirm="confirmDelete"
    @cancel="confirmDeleteKey = ''"
  />
</template>
