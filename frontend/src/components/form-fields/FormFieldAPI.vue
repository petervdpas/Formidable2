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

import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import APIFieldPicker from "./APIFieldPicker.vue";
import {
  Service as DataproviderSvc,
  type APIFieldDrift,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/dataprovider";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

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

// Display row label — falls back to field.label if Map.label is empty.
function rowLabel(idx: number, key: string): string {
  const m = props.field.map?.[idx];
  return m?.label?.trim() || key;
}

// Format a value for read-only display. JSON-flattened strings
// (from non-scalar source columns) display verbatim — they're
// already strings.
function display(v: any): string {
  if (v == null) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
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
            {{ display(value?.[m.key]) }}
          </dd>
        </template>
        <template v-if="!(field.map ?? []).length">
          <dt class="muted">—</dt>
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
