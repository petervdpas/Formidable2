<script setup lang="ts">
// Form-side renderer for an api-client (remote reference) field. The value is a
// stored snapshot: {id, label, fields, fetched}, or a list of them when the
// field is multi. That copy is the display truth. A live call refreshes it and
// never blanks it: an unreachable service, a retired record or a peer without
// the client all keep showing what was fetched last.
//
// modelValue: a snapshot object, an array of them, or null.

import { computed, inject, ref } from "vue";
import { useI18n } from "vue-i18n";
import APIClientPicker from "./APIClientPicker.vue";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { useAPIClients } from "../../composables/useAPIClients";
import { useToast } from "../../composables/useToast";
import { FORM_VALUES_KEY } from "../../composables/formValues";

interface Snapshot {
  id: string;
  label?: string;
  fields?: Record<string, unknown>;
  fetched?: string;
}

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();
const toast = useToast();
const clients = useAPIClients();

// Sibling values, so a param declared as field_key narrows the remote call to
// the record being edited. Absent outside StorageWorkspace (a plugin surface
// renders rows in isolation); such a param then resolves to empty.
const formValues = inject(FORM_VALUES_KEY, null);

const pickerOpen = ref(false);
const refreshing = ref("");

const multi = computed(() => props.field.multiple === true);
const mapKeys = computed<string[]>(() =>
  (props.field.map ?? []).map((m) => m.key).filter(Boolean),
);

const picks = computed<Snapshot[]>(() => {
  const v = props.modelValue;
  if (Array.isArray(v)) return v.filter(isSnapshot);
  if (isSnapshot(v)) return [v];
  return [];
});

function isSnapshot(v: unknown): v is Snapshot {
  return !!v && typeof v === "object" && typeof (v as Snapshot).id === "string";
}

function emitPicks(next: Snapshot[]) {
  emit("update:modelValue", multi.value ? next : (next[0] ?? null));
}

// Runtime parameters, resolved per call: a literal, or the current value of
// another field in this record.
const params = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {};
  for (const p of props.field.params ?? []) {
    if (!p.name) continue;
    if (p.field_key) {
      const v = formValues?.values.value?.[p.field_key];
      out[p.name] = v == null ? "" : String(v);
      continue;
    }
    out[p.name] = p.value ?? "";
  }
  return out;
});

function labelFor(m: string): string {
  const row = (props.field.map ?? []).find((r) => r.key === m);
  return row?.label?.trim() || m;
}

function display(v: unknown): string {
  if (v == null) return "";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

// Go's zero time renders as year 1; treat anything that old as "never".
function fetchedLabel(s: Snapshot): string {
  if (!s.fetched) return t("workspace.storage.api_client_field.never_fetched");
  const d = new Date(s.fetched);
  if (Number.isNaN(d.getTime()) || d.getUTCFullYear() <= 1) {
    return t("workspace.storage.api_client_field.never_fetched");
  }
  return d.toLocaleString();
}

async function onPicked(id: string) {
  pickerOpen.value = false;
  const res = await fetchSnapshot(id);
  if (!res) return;
  const next = multi.value
    ? [...picks.value.filter((p) => p.id !== res.id), res]
    : [res];
  emitPicks(next);
}

async function refresh(pick: Snapshot) {
  refreshing.value = pick.id;
  const res = await fetchSnapshot(pick.id);
  refreshing.value = "";
  // A failed refresh keeps the stored copy: that is the point of the field.
  if (!res) return;
  emitPicks(picks.value.map((p) => (p.id === pick.id ? res : p)));
}

async function fetchSnapshot(id: string): Promise<Snapshot | null> {
  const res = await clients.fetchSnapshot({
    connection: props.field.client_id ?? "",
    resource: props.field.resource ?? "",
    id,
    select: mapKeys.value,
    params: params.value,
  });
  if (!res.ok || !res.snapshot) {
    toast.error(res.message || "workspace.storage.api_client_field.fetch_failed");
    return null;
  }
  return res.snapshot as Snapshot;
}

function remove(pick: Snapshot) {
  emitPicks(picks.value.filter((p) => p.id !== pick.id));
}

const bound = computed(() => !!props.field.client_id && !!props.field.resource);
const pickLabel = computed(() =>
  multi.value && picks.value.length
    ? t("workspace.storage.api_client_field.pick_another")
    : picks.value.length
      ? t("workspace.storage.api_client_field.repick")
      : t("workspace.storage.api_client_field.pick"),
);
</script>

<template>
  <div class="api-client-field">
    <p v-if="!bound" class="muted small">
      {{ t('workspace.storage.api_client_field.unbound') }}
    </p>

    <template v-else>
      <section v-for="pick in picks" :key="pick.id" class="api-client-card">
        <header class="api-client-card-head">
          <span class="api-client-card-title">{{ pick.label || pick.id }}</span>
          <span class="api-client-actions">
            <button
              type="button"
              class="tool-btn small"
              :disabled="refreshing === pick.id"
              :title="t('workspace.storage.api_client_field.refresh')"
              @click="refresh(pick)"
            >
              {{ refreshing === pick.id
                ? t('shell.common.loading')
                : t('workspace.storage.api_client_field.refresh') }}
            </button>
            <button
              type="button"
              class="tool-btn small danger"
              :title="t('workspace.storage.api_client_field.remove')"
              @click="remove(pick)"
            >✕</button>
          </span>
        </header>

        <dl v-if="mapKeys.length" class="api-client-rows">
          <template v-for="m in mapKeys" :key="m">
            <dt>{{ labelFor(m) }}</dt>
            <dd>{{ display(pick.fields?.[m]) }}</dd>
          </template>
        </dl>

        <p class="api-client-stamp muted small">
          {{ t('workspace.storage.api_client_field.fetched_at', [fetchedLabel(pick)]) }}
        </p>
      </section>

      <div class="api-client-empty">
        <span v-if="!picks.length" class="muted small">
          {{ t('workspace.storage.api_client_field.empty') }}
        </span>
        <button type="button" class="tool-btn primary small" @click="pickerOpen = true">
          {{ pickLabel }}
        </button>
      </div>
    </template>

    <APIClientPicker
      :open="pickerOpen"
      :client-id="field.client_id ?? ''"
      :resource="field.resource ?? ''"
      :select="mapKeys"
      :params="params"
      @close="pickerOpen = false"
      @pick="onPicked"
    />
  </div>
</template>
