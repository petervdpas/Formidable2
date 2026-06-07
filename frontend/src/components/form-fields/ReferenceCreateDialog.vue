<script setup lang="ts">
// Create-and-link dialog for an api (relation reference) field. Renders the
// SUBSET of the target template's fields the field declares (its Map), plus a
// datafile-name input, and saves a new FIRST-CLASS record into the target
// collection (owned by the target). The new record's id is emitted back so the
// host field can link it. The record's other fields are completed by going to
// the target later ("Go to B").
//
// Reuses the form renderer for the subset widgets and Modal for the shell.

import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../Modal.vue";
import { TextField } from "../fields";
import FormFieldRenderer from "./FormFieldRenderer.vue";
import {
  Service as TemplateSvc,
  type Field,
  type APIMap,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import {
  Service as FormSvc,
  SavePayload,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { backendErrMessage } from "../../utils/backendError";
import { useToast } from "../../composables/useToast";

const props = defineProps<{
  open: boolean;
  /** Target template filename ("people.yaml"). */
  targetTemplate: string;
  /** The field's Map: the subset of target fields to edit here. */
  columns: APIMap[];
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "created", id: string): void;
}>();

const { t } = useI18n();
const toast = useToast();

const datafile = ref("");
const values = ref<Record<string, unknown>>({});
const targetFields = ref<Field[]>([]);
const saving = ref(false);

// Resolve the target template's Field definitions for the Map keys, in Map order.
async function reset() {
  datafile.value = "";
  values.value = {};
  targetFields.value = [];
  if (!props.targetTemplate) return;
  try {
    const tpl = await TemplateSvc.LoadTemplate(props.targetTemplate);
    const byKey: Record<string, Field> = {};
    for (const f of tpl?.fields ?? []) if (f.key) byKey[f.key] = f;
    const picked: Field[] = [];
    for (const m of props.columns ?? []) {
      const f = byKey[m.key];
      if (f) picked.push(f);
    }
    targetFields.value = picked;
  } catch (e) {
    toast.error(backendErrMessage(e));
  }
}

watch(
  () => [props.open, props.targetTemplate],
  () => {
    if (props.open) void reset();
  },
  { immediate: true },
);

function labelFor(field: Field): string {
  const m = (props.columns ?? []).find((c) => c.key === field.key);
  return m?.label?.trim() || field.label || field.key;
}

const canSave = computed(() => datafile.value.trim().length > 0 && !saving.value);

async function create() {
  const df = datafile.value.trim();
  if (!df) return;
  saving.value = true;
  try {
    const view = await FormSvc.SaveValues(
      props.targetTemplate,
      SavePayload.createFrom({ datafile: df, values: { ...values.value } }),
    );
    const id = view?.meta?.id ?? "";
    if (!id) {
      toast.error("status.template.load.failed", [df]);
      return;
    }
    emit("created", id);
    emit("close");
  } catch (e) {
    toast.error(backendErrMessage(e));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.storage.api_field.create_title')"
    width="560px"
    @close="emit('close')"
  >
    <div class="api-create-dialog">
      <div class="api-create-field-row">
        <label class="api-create-label">
          {{ t('workspace.storage.api_field.create_datafile') }}
        </label>
        <TextField
          v-model="datafile"
          :placeholder="t('workspace.storage.api_field.create_datafile_placeholder')"
        />
      </div>

      <div v-if="targetFields.length" class="api-create-fields">
        <div v-for="f in targetFields" :key="f.key" class="api-create-field-row">
          <label class="api-create-label">{{ labelFor(f) }}</label>
          <FormFieldRenderer
            :field="f"
            :model-value="values[f.key]"
            @update:model-value="(v: unknown) => (values[f.key] = v)"
          />
        </div>
      </div>
      <p v-else class="muted small">
        {{ t('workspace.storage.api_field.create_no_columns') }}
      </p>

      <p class="muted small">{{ t('workspace.storage.api_field.create_hint') }}</p>
    </div>

    <template #footer>
      <button type="button" class="tool-btn" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button type="button" class="tool-btn primary" :disabled="!canSave" @click="create">
        {{ t('workspace.storage.api_field.create_confirm') }}
      </button>
    </template>
  </Modal>
</template>
