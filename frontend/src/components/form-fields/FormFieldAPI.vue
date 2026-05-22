<script setup lang="ts">
// Form-side renderer for an api field: an embedded section showing
// the projected columns of one source record (read-only), plus a
// Pick button (opens APIFieldPicker by guid) and a Refresh button
// (refetch + drift detection via dataprovider.RefetchAPIFieldRow).
//
// modelValue shape:
//   null/undefined          → no record picked yet (empty state)
//   { guid, ...columnKey: value } → a stamped projection
//
// Multiplicity is NOT this component's job: a single api field is
// always one record's projection. Multiple records = wrap the field
// in a loopstart/loopstop pair (the existing loop renderer iterates
// the api field per iteration without any special-casing here).

import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import APIFieldPicker from "./APIFieldPicker.vue";
import {
  Service as DataproviderSvc,
  type APIFieldDrift,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import {
  Service as TemplateSvc,
  type Field,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

// ── Local state ────────────────────────────────────────────────────────
const pickerOpen = ref(false);
const fetching = ref(false);
const error = ref("");
const drift = ref<APIFieldDrift[]>([]);

// ── Derived from the bound field/value ─────────────────────────────────
const sourceTemplate = computed(() => props.field.collection ?? "");
const columnKeys = computed<string[]>(() =>
  (props.field.map ?? []).map((m) => m.key).filter(Boolean),
);

const value = computed<Record<string, any> | null>(() => {
  const v = props.modelValue;
  if (v == null || typeof v !== "object" || Array.isArray(v)) return null;
  return v as Record<string, any>;
});

const guid = computed(() => (value.value?.guid as string) ?? "");

// ── Picker → fetch → stamp ─────────────────────────────────────────────
async function onPick(picked: string) {
  if (!sourceTemplate.value || !picked) return;
  await stampGuid(picked);
}

async function stampGuid(g: string) {
  fetching.value = true;
  error.value = "";
  drift.value = [];
  try {
    const res = await DataproviderSvc.FetchAPIFieldRow(
      sourceTemplate.value,
      g,
      columnKeys.value,
    );
    if (res.kind) {
      error.value = res.message || res.kind;
      return;
    }
    emit("update:modelValue", { guid: g, ...(res.row ?? {}) });
  } catch (e) {
    error.value = String((e as Error)?.message ?? e);
  } finally {
    fetching.value = false;
  }
}

// ── Refetch / drift ────────────────────────────────────────────────────
async function onRefresh() {
  if (!guid.value) return;
  fetching.value = true;
  error.value = "";
  drift.value = [];
  try {
    const stored: Record<string, any> = {};
    if (value.value) {
      for (const k of columnKeys.value) stored[k] = value.value[k];
    }
    const res = await DataproviderSvc.RefetchAPIFieldRow(
      sourceTemplate.value,
      guid.value,
      columnKeys.value,
      stored,
    );
    if (res.kind) {
      error.value = res.message || res.kind;
      return;
    }
    drift.value = res.drift ?? [];
  } catch (e) {
    error.value = String((e as Error)?.message ?? e);
  } finally {
    fetching.value = false;
  }
}

function applyDrift() {
  if (!value.value || drift.value.length === 0) return;
  const next = { ...value.value };
  for (const d of drift.value) {
    next[d.key] = d.current;
  }
  emit("update:modelValue", next);
  drift.value = [];
}

function dismissDrift() {
  drift.value = [];
}

function clear() {
  emit("update:modelValue", null);
  drift.value = [];
  error.value = "";
}

// Display row label - falls back to field.label if Map.label is empty.
function rowLabel(idx: number, key: string): string {
  const m = props.field.map?.[idx];
  return m?.label?.trim() || key;
}

// Format a value for read-only display. JSON-flattened strings
// (from non-scalar source columns) display verbatim - they're
// already strings.
function display(v: any): string {
  if (v == null) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

// ── Source-aware rich rendering ────────────────────────────────────────
//
// Source values arrive in their native JSON shape (the backend stamps
// scalars/arrays/maps verbatim; the host's .meta.json is itself JSON,
// so no string-encoding gymnastics are needed). The card just picks
// the right widget based on the source field's type - `tags` → chip
// list, `list` → bullet list, `table` → small <table>. Anything else
// falls through to the verbatim text representation.
//
// We keep the FULL source Field around (not just the type) so the
// table renderer can read `field.options[].label` for proper column
// headers (FormFieldTable stores rows as arrays-of-arrays - the
// header labels live in the field's options metadata).

const sourceFields = ref<Record<string, Field>>({});

async function loadSourceFields(filename: string) {
  if (!filename) {
    sourceFields.value = {};
    return;
  }
  try {
    const tpl = await TemplateSvc.LoadTemplate(filename);
    if (!tpl) return;
    const map: Record<string, Field> = {};
    for (const f of tpl.fields ?? []) {
      if (f.key) map[f.key] = f;
    }
    sourceFields.value = map;
  } catch {
    sourceFields.value = {};
  }
}

watch(
  () => props.field.collection,
  (collection) => {
    void loadSourceFields(collection ?? "");
  },
  { immediate: true },
);

type RenderedShape =
  | { kind: "text"; text: string }
  | { kind: "tags"; items: string[] }
  | { kind: "list"; items: any[] }
  | { kind: "table"; headers: string[] | null; rows: any[][] };

function shapeFor(raw: any, sourceField: Field | undefined): RenderedShape {
  if (raw == null) return { kind: "text", text: "" };
  const sourceType = sourceField?.type ?? "";
  if (sourceType === "tags" && Array.isArray(raw)) {
    return { kind: "tags", items: raw.map(String) };
  }
  if (sourceType === "list" && Array.isArray(raw)) {
    return { kind: "list", items: raw };
  }
  if ((sourceType === "table" || sourceType === "multioption") && Array.isArray(raw)) {
    if (raw.length === 0) return { kind: "text", text: "" };
    return shapeTable(raw, sourceField);
  }
  return { kind: "text", text: display(raw) };
}

// headersForTableField reads `field.options[]` (the column metadata
// FormFieldTable also reads) and returns the user-facing labels, in
// the same positional order the stored rows use. Falls back to the
// option's `value` key when label is missing.
function headersForTableField(field: Field | undefined): string[] | null {
  const opts = field?.options;
  if (!Array.isArray(opts) || opts.length === 0) return null;
  const headers: string[] = [];
  for (const o of opts) {
    if (o && typeof o === "object") {
      const rec = o as Record<string, unknown>;
      const label = typeof rec.label === "string" && rec.label.trim() !== ""
        ? rec.label
        : typeof rec.value === "string"
          ? rec.value
          : "";
      headers.push(label);
    } else {
      headers.push("");
    }
  }
  return headers;
}

// shapeTable handles both array-of-objects (keys → headers) and
// array-of-arrays (positional cells; headers come from the source
// field's options metadata).
function shapeTable(rows: any[], sourceField: Field | undefined): RenderedShape {
  const first = rows[0];
  if (Array.isArray(first)) {
    const headers = headersForTableField(sourceField);
    // Pad/trim each row so every row has the same arity as headers.
    const arity = headers?.length ?? Math.max(...rows.map((r) => Array.isArray(r) ? r.length : 0));
    return {
      kind: "table",
      headers,
      rows: rows.map((r) => {
        const arr = Array.isArray(r) ? r.slice() : [];
        while (arr.length < arity) arr.push("");
        return arr.slice(0, arity);
      }),
    };
  }
  if (first && typeof first === "object") {
    // Prefer source-field options for header LABELS; fall back to keys
    // of the first row when options are missing.
    const headers = headersForTableField(sourceField);
    const keys = headers
      ? // map keys are still the option .value entries - read them from options
        ((sourceField?.options ?? []) as Array<Record<string, unknown>>)
          .map((o) => (o && typeof o === "object" ? String(o.value ?? "") : ""))
      : Object.keys(first);
    const labels = headers ?? keys;
    return {
      kind: "table",
      headers: labels,
      rows: rows.map((r) => keys.map((k) => (r as any)?.[k])),
    };
  }
  // Array of scalars - render as a 1-column table.
  return { kind: "table", headers: null, rows: rows.map((r) => [r]) };
}

// Build a "drifted?" lookup for cell-level highlighting.
const driftedKeys = computed(() => {
  const set = new Set<string>();
  for (const d of drift.value) set.add(d.key);
  return set;
});
</script>

<template>
  <div class="api-field">
    <!-- Empty state -->
    <div v-if="!guid" class="api-field-empty">
      <span class="muted small">
        {{ t("workspace.storage.api_field.empty") }}
      </span>
      <button
        type="button"
        class="tool-btn primary small"
        :disabled="!sourceTemplate || fetching"
        @click="pickerOpen = true"
      >
        {{ t("workspace.storage.api_field.pick") }}
      </button>
      <p v-if="!sourceTemplate" class="muted small">
        {{ t("workspace.storage.api_field.no_source_configured") }}
      </p>
    </div>

    <!-- Stamped state -->
    <section v-else class="api-field-card" :class="{ drifted: drift.length > 0 }">
      <header class="api-field-card-head">
        <span class="api-field-card-title">
          {{ field.label || field.key }}
          <span class="muted small">({{ sourceTemplate }})</span>
        </span>
        <span class="api-field-actions">
          <button
            type="button"
            class="tool-btn small"
            :disabled="fetching"
            @click="onRefresh"
            :title="t('workspace.storage.api_field.refresh')"
          >
            ↻
          </button>
          <button
            type="button"
            class="tool-btn small"
            :disabled="fetching"
            @click="pickerOpen = true"
            :title="t('workspace.storage.api_field.repick')"
          >
            {{ t("workspace.storage.api_field.repick") }}
          </button>
          <button
            type="button"
            class="tool-btn small danger"
            :disabled="fetching"
            @click="clear"
            :title="t('workspace.storage.api_field.clear')"
          >
            ✕
          </button>
        </span>
      </header>

      <!-- Drift banner -->
      <div v-if="drift.length > 0" class="api-field-drift-banner">
        <span class="small">
          {{ t("workspace.storage.api_field.drift_detected", { n: drift.length }) }}
        </span>
        <span>
          <button type="button" class="tool-btn small primary" @click="applyDrift">
            {{ t("workspace.storage.api_field.drift_apply") }}
          </button>
          <button type="button" class="tool-btn small" @click="dismissDrift">
            {{ t("common.cancel") }}
          </button>
        </span>
      </div>

      <dl class="api-field-rows">
        <template v-for="(m, idx) in field.map ?? []" :key="m.key + ':' + idx">
          <dt>{{ rowLabel(idx, m.key) }}</dt>
          <dd :class="{ drifted: driftedKeys.has(m.key) }">
            <template v-for="(s, _i) in [shapeFor(value?.[m.key], sourceFields[m.key])]" :key="m.key">
              <!-- Tags → chip list -->
              <span v-if="s.kind === 'tags'" class="api-cell-tags">
                <span v-for="tag in s.items" :key="tag" class="tag-chip">{{ tag }}</span>
                <span v-if="s.items.length === 0" class="muted small">-</span>
              </span>
              <!-- List → bullets -->
              <ul v-else-if="s.kind === 'list'" class="api-cell-list">
                <li v-for="(item, i) in s.items" :key="i">{{ display(item) }}</li>
                <li v-if="s.items.length === 0" class="muted small">-</li>
              </ul>
              <!-- Table → small grid -->
              <table v-else-if="s.kind === 'table'" class="api-cell-table">
                <thead v-if="s.headers">
                  <tr>
                    <th v-for="h in s.headers" :key="h">{{ h }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, ri) in s.rows" :key="ri">
                    <td v-for="(cell, ci) in row" :key="ci">{{ display(cell) }}</td>
                  </tr>
                </tbody>
              </table>
              <!-- Fallback → text -->
              <span v-else>{{ s.text }}</span>
            </template>
          </dd>
        </template>
        <template v-if="!(field.map ?? []).length">
          <dt class="muted">-</dt>
          <dd class="muted small">
            {{ t("workspace.storage.api_field.no_columns_configured") }}
          </dd>
        </template>
      </dl>

      <p class="api-field-guid muted small">guid: {{ guid }}</p>
    </section>

    <p v-if="error" class="error small">{{ error }}</p>

    <APIFieldPicker
      :open="pickerOpen"
      :source-template="sourceTemplate"
      @close="pickerOpen = false"
      @pick="onPick"
    />
  </div>
</template>

<style scoped>
.api-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.api-field-empty {
  display: flex;
  align-items: center;
  gap: 8px;
}
.api-field-card {
  border: 1px solid color-mix(in oklab, currentColor 25%, transparent);
  border-radius: 6px;
  padding: 8px 10px;
  background: color-mix(in oklab, currentColor 4%, transparent);
}
.api-field-card.drifted {
  border-color: color-mix(in oklab, var(--color-warn, #f39c12) 80%, transparent);
}
.api-field-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}
.api-field-card-title {
  font-weight: 600;
}
.api-field-actions {
  display: inline-flex;
  gap: 4px;
}
.api-field-rows {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
  margin: 0;
}
.api-field-rows dt {
  font-weight: 500;
  opacity: 0.85;
}
.api-field-rows dd {
  margin: 0;
  word-break: break-word;
}
.api-field-rows dd.drifted {
  background: color-mix(in oklab, var(--color-warn, #f39c12) 18%, transparent);
  padding: 0 4px;
  border-radius: 4px;
}
.api-cell-tags {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}
.tag-chip {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, currentColor 14%, transparent);
  font-size: 0.8rem;
}
.api-cell-list {
  margin: 0;
  padding-left: 18px;
}
.api-cell-list li {
  list-style: disc;
}
.api-cell-table {
  border-collapse: collapse;
  font-size: 0.85rem;
  width: auto;
}
.api-cell-table th,
.api-cell-table td {
  padding: 2px 8px;
  border: 1px solid color-mix(in oklab, currentColor 18%, transparent);
  text-align: left;
  vertical-align: top;
  word-break: break-word;
}
.api-cell-table th {
  background: color-mix(in oklab, currentColor 8%, transparent);
  font-weight: 600;
}
.api-field-drift-banner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: color-mix(in oklab, var(--color-warn, #f39c12) 12%, transparent);
  padding: 4px 8px;
  border-radius: 4px;
  margin-bottom: 6px;
}
.api-field-guid {
  font-family: ui-monospace, SFMono-Regular, monospace;
  font-size: 0.7rem;
  margin: 6px 0 0;
}
.tool-btn.danger:hover {
  background: color-mix(in oklab, var(--color-danger, #c0392b) 25%, transparent);
}
.error {
  color: var(--color-danger, #c0392b);
}
</style>
