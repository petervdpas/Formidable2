<script setup lang="ts">
// Editor for the api-client (remote reference) field's config:
//   • client_id - which app-level API client to call.
//   • resource  - which of its resources to list and fetch from.
//   • map       - the subset of the resource's projected fields to STORE.
//   • multiple  - one pick or a list.
//
// Unlike the api field, the picked values are written into the record. The
// client, its spec file and its vault secret live outside the synced tree, so a
// peer pulling the repo can only see what the field stored.

import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, SwitchField, TextField } from "./fields";
import {
  APIMap,
  APIParam,
  type Field,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { Resource } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";
import { useAPIClients } from "../composables/useAPIClients";

const props = defineProps<{
  /** Bound field draft, mutated in place so FieldEditModal's copy-on-open /
   *  commit-on-confirm cycle works without per-attribute emits. */
  field: Field;
  /** Sibling field keys, so a parameter can read another field of the record. */
  siblingKeys?: string[];
}>();

const { t } = useI18n();
const clients = useAPIClients();

onMounted(() => void clients.refresh());

const clientOptions = computed(() =>
  clients.summaries.value.map((c) => ({ value: c.id, label: c.name || c.id })),
);

// The chosen client's resources. Loaded on demand: the summary list carries a
// count, not the bindings themselves.
const resources = ref<Resource[]>([]);
const loadingResources = ref(false);
const loadError = ref("");

async function loadResources(id: string) {
  resources.value = [];
  loadError.value = "";
  if (!id) return;
  loadingResources.value = true;
  const res = await clients.select(id);
  loadingResources.value = false;
  if (!res.ok) {
    loadError.value = res.message;
    return;
  }
  resources.value = clients.detail.value?.client.resources ?? [];
}

watch(() => props.field.client_id, (id) => void loadResources(id ?? ""), {
  immediate: true,
});

const resourceOptions = computed(() =>
  resources.value.map((r) => ({ value: r.key, label: r.label || r.key })),
);

const selectedResource = computed<Resource | null>(
  () => resources.value.find((r) => r.key === props.field.resource) ?? null,
);

// The fields the resource declares are exactly what a pick can store.
const projectableOptions = computed(() =>
  (selectedResource.value?.fields ?? []).map((f) => ({
    value: f.key,
    label: f.label ? `${f.label} (${f.key})` : f.key,
  })),
);

// ── Runtime parameters ─────────────────────────────────────────────────
// The client binding holds what is always true of the service. These hold what
// THIS field asks for, resolved per call: a literal, or another field's value.
function ensureParams(): APIParam[] {
  if (!Array.isArray(props.field.params)) props.field.params = [];
  return props.field.params;
}
function addParam() {
  ensureParams().push(APIParam.createFrom({ name: "", value: "", field_key: "" }));
}
function removeParam(idx: number) {
  ensureParams().splice(idx, 1);
}

const siblingOptions = computed(() => [
  { value: "", label: t("workspace.templates.api_client_editor.param.literal") },
  ...(props.siblingKeys ?? []).map((k) => ({ value: k, label: k })),
]);

// A row is either a literal or a field reference; picking one clears the other,
// so the draft can never carry the ambiguous both-set shape the backend rejects.
function onParamSource(row: APIParam, key: string) {
  row.field_key = key;
  if (key) row.value = "";
}

function ensureMap(): APIMap[] {
  if (!Array.isArray(props.field.map)) props.field.map = [];
  return props.field.map;
}
function addRow() {
  ensureMap().push(APIMap.createFrom({ key: "", label: "" }));
}
function removeRow(idx: number) {
  ensureMap().splice(idx, 1);
}

// Switching client invalidates the resource and the projection: they name
// things that only exist inside the previous client.
function onClient(id: string) {
  if (id === props.field.client_id) return;
  props.field.client_id = id;
  props.field.resource = "";
  props.field.map = [];
}
function onResource(key: string) {
  if (key === props.field.resource) return;
  props.field.resource = key;
  props.field.map = [];
}

const noClients = computed(
  () => !clients.loading.value && clientOptions.value.length === 0,
);
</script>

<template>
  <div class="api-client-editor">
    <div class="api-client-editor-row">
      <label class="api-client-editor-label">
        {{ t("workspace.templates.api_client_editor.client") }}
      </label>
      <div class="api-client-editor-control">
        <SelectField
          :model-value="field.client_id ?? ''"
          :options="clientOptions"
          :placeholder="t('workspace.templates.api_client_editor.client_placeholder')"
          :disabled="clientOptions.length === 0"
          @update:model-value="onClient"
        />
        <p v-if="noClients" class="muted small">
          {{ t("workspace.templates.api_client_editor.no_clients") }}
        </p>
        <p v-if="loadError" class="error small">{{ loadError }}</p>
      </div>
    </div>

    <div class="api-client-editor-row">
      <label class="api-client-editor-label">
        {{ t("workspace.templates.api_client_editor.resource") }}
      </label>
      <div class="api-client-editor-control">
        <SelectField
          :model-value="field.resource ?? ''"
          :options="resourceOptions"
          :placeholder="
            loadingResources
              ? t('shell.common.loading')
              : t('workspace.templates.api_client_editor.resource_placeholder')
          "
          :disabled="loadingResources || resourceOptions.length === 0"
          @update:model-value="onResource"
        />
        <p
          v-if="field.client_id && !loadingResources && resourceOptions.length === 0"
          class="muted small"
        >
          {{ t("workspace.templates.api_client_editor.no_resources") }}
        </p>
      </div>
    </div>

    <div class="api-client-editor-row">
      <label class="api-client-editor-label">
        {{ t("workspace.templates.api_client_editor.multiple") }}
      </label>
      <div class="api-client-editor-control">
        <SwitchField v-model="field.multiple" />
        <p class="muted small">
          {{ t("workspace.templates.api_client_editor.multiple_hint") }}
        </p>
      </div>
    </div>

    <div class="api-client-editor-row">
      <label class="api-client-editor-label">
        {{ t("workspace.templates.api_client_editor.params") }}
      </label>
      <div class="api-client-editor-control">
        <p class="muted small">
          {{ t("workspace.templates.api_client_editor.params_hint") }}
        </p>
        <table v-if="(field.params ?? []).length" class="api-client-map-table">
          <thead>
            <tr>
              <th>{{ t("workspace.templates.api_client_editor.param.name") }}</th>
              <th>{{ t("workspace.templates.api_client_editor.param.source") }}</th>
              <th>{{ t("workspace.templates.api_client_editor.param.value") }}</th>
              <th class="api-client-col-actions"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, i) in field.params ?? []" :key="i">
              <td><TextField v-model="row.name" placeholder="$filter" /></td>
              <td>
                <SelectField
                  :model-value="row.field_key ?? ''"
                  :options="siblingOptions"
                  @update:model-value="(k: string) => onParamSource(row, k)"
                />
              </td>
              <td>
                <TextField v-if="!row.field_key" v-model="row.value" />
                <span v-else class="muted small">
                  {{ t("workspace.templates.api_client_editor.param.from_field") }}
                </span>
              </td>
              <td class="api-client-col-actions">
                <button
                  type="button"
                  class="tool-btn small danger"
                  :title="t('workspace.templates.api_client_editor.param.remove')"
                  @click="removeParam(i)"
                >−</button>
              </td>
            </tr>
          </tbody>
        </table>
        <button
          type="button"
          class="btn-ghost-block"
          :disabled="!field.resource"
          :title="t('workspace.templates.api_client_editor.param.add')"
          @click="addParam"
        >+</button>
      </div>
    </div>

    <div class="api-client-editor-row">
      <label class="api-client-editor-label">
        {{ t("workspace.templates.api_client_editor.stored") }}
      </label>
      <div class="api-client-editor-control">
        <p class="muted small">
          {{ t("workspace.templates.api_client_editor.stored_hint") }}
        </p>
        <table v-if="(field.map ?? []).length" class="api-client-map-table">
          <thead>
            <tr>
              <th>{{ t("workspace.templates.api_client_editor.col.key") }}</th>
              <th>{{ t("workspace.templates.api_client_editor.col.label") }}</th>
              <th class="api-client-col-actions"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, i) in field.map ?? []" :key="i">
              <td>
                <SelectField
                  v-model="row.key"
                  :options="projectableOptions"
                  :disabled="projectableOptions.length === 0"
                />
              </td>
              <td><TextField v-model="row.label" /></td>
              <td class="api-client-col-actions">
                <button
                  type="button"
                  class="tool-btn small danger"
                  :title="t('workspace.templates.api_client_editor.remove_column')"
                  @click="removeRow(i)"
                >−</button>
              </td>
            </tr>
          </tbody>
        </table>
        <button
          type="button"
          class="btn-ghost-block"
          :disabled="!field.resource"
          :title="t('workspace.templates.api_client_editor.add_column')"
          @click="addRow"
        >+</button>
        <p v-if="field.resource && projectableOptions.length === 0" class="muted small">
          {{ t("workspace.templates.api_client_editor.no_projectable") }}
        </p>
      </div>
    </div>
  </div>
</template>
