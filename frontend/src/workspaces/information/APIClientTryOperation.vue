<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SelectField, TextField } from "../../components/fields";
import type {
  OperationInfo,
  TryForm,
  TryResult,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();

const props = defineProps<{
  operations: OperationInfo[];
  form: TryForm | null;
  result: TryResult | null;
  running: boolean;
  /** A refusal that happened before the call, so there is no result to show. */
  error: string;
}>();

const emit = defineEmits<{
  (e: "pick", operation: string): void;
  (e: "run", params: Record<string, string>): void;
}>();

const operation = ref("");

// Values are keyed by parameter name, which is unique per operation across
// locations, so one map covers path, query, header and cookie alike.
const values = ref<Record<string, string>>({});

const operationOptions = computed(() =>
  props.operations.map((info) => ({
    value: info.operation.id,
    label: `${info.operation.method} ${info.operation.path}`,
  })),
);

// Default to the first operation, so the pane is never an empty picker with
// nothing to look at.
watch(
  () => props.operations,
  (ops) => {
    if (!operation.value && ops.length > 0) {
      operation.value = ops[0].operation.id;
      emit("pick", operation.value);
    }
  },
  { immediate: true },
);

watch(operation, (id) => {
  if (id) emit("pick", id);
});

// Re-seed from the form whenever it arrives, so what a resource already fixes
// is pre-filled and the console reproduces the call a field would make.
watch(
  () => props.form,
  (form) => {
    const next: Record<string, string> = {};
    for (const p of form?.params ?? []) next[p.name] = p.value ?? "";
    values.value = next;
  },
);

const params = computed(() => props.form?.params ?? []);

const missing = computed(() =>
  params.value.filter((p) => p.required && !(values.value[p.name] ?? "").trim()),
);

const canRun = computed(
  () => !!props.form?.runnable && !props.running && missing.value.length === 0,
);

// The reason set is closed, so the key is declared rather than interpolated.
const REASON_KEYS: Record<string, string> = {
  method_not_allowed: "apiclients.try.not_runnable",
};

const notRunnable = computed(() => {
  const reason = props.form?.reason;
  if (!reason) return "";
  const key = REASON_KEYS[reason];
  return key ? t(key) : reason;
});

const statusClass = computed(() => {
  if (!props.result) return "";
  return props.result.failed ? "apiclients-try-status failed" : "apiclients-try-status ok";
});

function run(): void {
  if (!canRun.value) return;
  const out: Record<string, string> = {};
  for (const [name, value] of Object.entries(values.value)) {
    if (value.trim()) out[name] = value;
  }
  emit("run", out);
}
</script>

<template>
  <div class="apiclients-block">
    <p class="muted small">{{ t('apiclients.try.hint') }}</p>

    <div class="apiclients-test-controls">
      <SelectField v-model="operation" :options="operationOptions" />
      <button class="tool-btn primary" type="button" :disabled="!canRun" @click="run">
        <i class="fa-solid fa-play" aria-hidden="true"></i>
        {{ running ? t('apiclients.try.running') : t('apiclients.try.run') }}
      </button>
    </div>

    <p v-if="notRunnable" class="apiclients-problems">{{ notRunnable }}</p>

    <table v-if="params.length > 0" class="apiclients-fields-table">
      <thead>
        <tr>
          <th>{{ t('apiclients.operations.param_name') }}</th>
          <th>{{ t('apiclients.operations.param_in') }}</th>
          <th>{{ t('apiclients.try.value') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in params" :key="p.in + p.name">
          <td>
            <code>{{ p.name }}</code>
            <span v-if="p.required" class="apiclients-try-required">*</span>
          </td>
          <td class="muted small">{{ p.in }}</td>
          <td><TextField v-model="values[p.name]" :placeholder="p.type" /></td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="form" class="muted small">{{ t('apiclients.try.no_params') }}</p>

    <p v-if="missing.length > 0" class="muted small">
      {{ t('apiclients.try.missing', [missing.map((p) => p.name).join(', ')]) }}
    </p>

    <p v-if="error" class="apiclients-error">
      <i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>
      {{ error }}
    </p>

    <template v-if="result">
      <div class="apiclients-try-head">
        <span :class="statusClass">{{ result.status }}</span>
        <span class="muted small">{{ result.status_text }}</span>
        <span class="muted small">{{ t('apiclients.try.duration', [String(result.duration_ms)]) }}</span>
        <span v-if="result.content_type" class="muted small">{{ result.content_type }}</span>
      </div>

      <code class="apiclients-try-url">{{ result.method }} {{ result.url }}</code>

      <p v-if="result.truncated" class="muted small">{{ t('apiclients.try.truncated') }}</p>

      <pre class="apiclients-try-body">{{ result.body }}</pre>
    </template>
  </div>
</template>
