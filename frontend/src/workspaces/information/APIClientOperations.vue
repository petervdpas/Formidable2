<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, TextField } from "../../components/fields";
import type {
  Connection,
  OperationInfo,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();

const props = defineProps<{
  operations: OperationInfo[];
  /** The client being edited, for the effective base URL shown per operation. */
  client: Connection | null;
  /** The first absolute server the document declares, used when no base URL is set. */
  fallbackServer: string;
  shapes: string[];
  loading: boolean;
}>();

const emit = defineEmits<{
  (e: "bind", value: OperationInfo): void;
}>();

const filter = ref("");
const shapeFilter = ref("");
const selectedID = ref("");

// A response shape is a closed set, so the labels are declared rather than
// built from the value: an interpolated key hides every one of these strings
// from the extractor.
const SHAPE_LABEL_KEYS: Record<string, string> = {
  records: "apiclients.operations.shape.records",
  keyed: "apiclients.operations.shape.keyed",
  values: "apiclients.operations.shape.values",
  keyed_values: "apiclients.operations.shape.keyed_values",
  record: "apiclients.operations.shape.record",
  unknown: "apiclients.operations.shape.unknown",
};

function shapeLabel(shape: string): string {
  const key = SHAPE_LABEL_KEYS[shape];
  return key ? t(key) : shape;
}

const ROLE_LABEL_KEYS: Record<string, string> = {
  list: "apiclients.resource.list_op",
  get: "apiclients.resource.get_op",
};

function roleLabel(role: string): string {
  const key = ROLE_LABEL_KEYS[role];
  return key ? t(key) : role;
}

const shapeOptions = computed(() => [
  { value: "", label: t("apiclients.operations.filter_all") },
  ...props.shapes.map((s) => ({ value: s, label: shapeLabel(s) })),
]);

const visible = computed(() => {
  const needle = filter.value.trim().toLowerCase();
  return props.operations.filter((info) => {
    if (shapeFilter.value && info.shape !== shapeFilter.value) return false;
    if (!needle) return true;
    const haystack = [
      info.operation.id,
      info.operation.path,
      info.operation.method,
      info.operation.summary ?? "",
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(needle);
  });
});

// Keep a selection that is still on screen, so filtering never leaves the
// detail pane describing a row the list no longer shows.
watch(visible, (rows) => {
  if (rows.length === 0) {
    selectedID.value = "";
    return;
  }
  if (!rows.some((r) => r.operation.id === selectedID.value)) {
    selectedID.value = rows[0].operation.id;
  }
});

const selected = computed(() =>
  props.operations.find((i) => i.operation.id === selectedID.value) ?? null,
);

const baseURL = computed(() => (props.client?.base_url || props.fallbackServer || "").replace(/\/$/, ""));

const fullURL = computed(() =>
  selected.value ? baseURL.value + selected.value.operation.path : "",
);

function methodClass(method: string): string {
  return "apiclients-method apiclients-method-" + method.toLowerCase();
}

function paramsOf(info: OperationInfo) {
  return info.operation.params ?? [];
}

function propertiesOf(info: OperationInfo) {
  return info.operation.result?.properties ?? [];
}
</script>

<template>
  <div class="apiclients-block">
    <p class="muted small">{{ t('apiclients.operations.hint') }}</p>

    <div class="apiclients-test-controls">
      <TextField v-model="filter" :placeholder="t('apiclients.operations.filter')" />
      <SelectField v-model="shapeFilter" :options="shapeOptions" />
      <span class="muted small">
        {{ t('apiclients.operations.count', [String(visible.length), String(operations.length)]) }}
      </span>
    </div>

    <p v-if="loading" class="muted small">{{ t('apiclients.operations.loading') }}</p>
    <p v-else-if="operations.length === 0" class="muted small">
      {{ t('apiclients.operations.empty') }}
    </p>
    <p v-else-if="visible.length === 0" class="muted small">
      {{ t('apiclients.operations.no_match') }}
    </p>

    <div v-else class="apiclients-ops">
      <ul class="apiclients-ops-list">
        <li
          v-for="info in visible"
          :key="info.operation.id"
          :class="['apiclients-ops-row', { selected: info.operation.id === selectedID }]"
          @click="selectedID = info.operation.id"
        >
          <span :class="methodClass(info.operation.method)">{{ info.operation.method }}</span>
          <span class="apiclients-ops-path">{{ info.operation.path }}</span>
          <span v-if="info.bound_by?.length" class="apiclients-ops-bound">
            <i class="fa-solid fa-link" aria-hidden="true"></i>
          </span>
        </li>
      </ul>

      <div v-if="selected" class="apiclients-ops-detail">
        <div class="apiclients-ops-detail-head">
          <span :class="methodClass(selected.operation.method)">
            {{ selected.operation.method }}
          </span>
          <code class="apiclients-ops-url">{{ fullURL }}</code>
        </div>

        <p v-if="selected.operation.summary" class="apiclients-ops-summary">
          {{ selected.operation.summary }}
        </p>

        <dl class="apiclients-ops-facts">
          <dt>{{ t('apiclients.operations.operation_id') }}</dt>
          <dd>
            <code>{{ selected.operation.id }}</code>
            <span v-if="selected.operation.synthetic" class="muted small">
              {{ t('apiclients.operations.synthetic') }}
            </span>
          </dd>

          <dt>{{ t('apiclients.operations.returns') }}</dt>
          <dd>
            {{ shapeLabel(selected.shape) }}
            <code v-if="selected.operation.result?.items_path">
              {{ selected.operation.result.items_path }}
            </code>
          </dd>

          <dt>{{ t('apiclients.operations.bound') }}</dt>
          <dd>
            <template v-if="selected.bound_by?.length">
              <span v-for="b in selected.bound_by" :key="b.resource + b.role" class="apiclients-chip">
                <code>{{ b.resource }}</code> · {{ roleLabel(b.role) }}
              </span>
            </template>
            <span v-else class="muted small">{{ t('apiclients.operations.unbound') }}</span>
          </dd>
        </dl>

        <template v-if="paramsOf(selected).length > 0">
          <h4 class="apiclients-ops-heading">{{ t('apiclients.operations.parameters') }}</h4>
          <table class="apiclients-fields-table">
            <thead>
              <tr>
                <th>{{ t('apiclients.operations.param_name') }}</th>
                <th>{{ t('apiclients.operations.param_in') }}</th>
                <th>{{ t('apiclients.operations.param_type') }}</th>
                <th>{{ t('apiclients.operations.param_required') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in paramsOf(selected)" :key="p.in + p.name">
                <td><code>{{ p.name }}</code></td>
                <td class="muted small">{{ p.in }}</td>
                <td class="muted small">{{ p.type || '-' }}</td>
                <td class="muted small">
                  {{ p.required ? t('apiclients.operations.required') : t('apiclients.operations.optional') }}
                </td>
              </tr>
            </tbody>
          </table>
        </template>

        <template v-if="propertiesOf(selected).length > 0">
          <h4 class="apiclients-ops-heading">{{ t('apiclients.operations.response_fields') }}</h4>
          <div class="apiclients-ops-props">
            <span v-for="p in propertiesOf(selected)" :key="p.pointer" class="apiclients-chip">
              <code>{{ p.pointer }}</code>
              <span v-if="p.type" class="muted small"> {{ p.type }}</span>
            </span>
          </div>
        </template>

        <div v-if="selected.collection" class="apiclients-actions">
          <button class="tool-btn primary" type="button" @click="emit('bind', selected)">
            <i class="fa-solid fa-plus" aria-hidden="true"></i>
            {{ t('apiclients.operations.bind') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
