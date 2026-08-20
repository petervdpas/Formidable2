<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "../../components/Modal.vue";
import { FormRow, FormSection, SelectField, TextField } from "../../components/fields";
import type {
  Catalog,
  Resource,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  /** The resource being edited, or null when adding a new one. */
  resource: Resource | null;
  /** Operations the uploaded document offers, for the operation pickers. */
  catalog: Catalog | null;
  keyStyles: string[];
  paginationStyles: string[];
  itemsModes: string[];
}>();

const emit = defineEmits<{
  (e: "apply", value: Resource): void;
  (e: "close"): void;
}>();

// The generated Resource marks the optional halves of a binding as optional,
// which is right on the wire and awkward in a form. The draft fills them in so
// every control has something to bind to, and toResource strips back on apply.
interface DraftField {
  key: string;
  pointer: string;
  label: string;
  type: string;
  remote: string;
}

interface DraftResource {
  key: string;
  label: string;
  list: { operation: string; params: Record<string, string> };
  get: { operation: string; params: Record<string, string> };
  items_path: string;
  items_mode: string;
  id_path: string;
  label_path: string;
  search_param: string;
  search_template: string;
  select_param: string;
  key_style: string;
  pagination: {
    style: string;
    limit_param: string;
    offset_param: string;
    cursor_param: string;
    cursor_path: string;
    link_path: string;
  };
  fields: DraftField[];
}

// A local working copy, so cancelling really cancels. Editing the prop in
// place would leave a half-typed binding behind on close.
const draft = ref<DraftResource>(blank());

function blank(): DraftResource {
  return {
    key: "",
    label: "",
    list: { operation: "", params: {} },
    get: { operation: "", params: {} },
    items_path: "",
    items_mode: "",
    id_path: "",
    label_path: "",
    search_param: "",
    search_template: "",
    select_param: "",
    key_style: "",
    pagination: {
      style: "none",
      limit_param: "",
      offset_param: "",
      cursor_param: "",
      cursor_path: "",
      link_path: "",
    },
    fields: [],
  };
}

/** Widen a stored resource into a fully-populated draft. */
function toDraft(r: Resource): DraftResource {
  const base = blank();
  return {
    ...base,
    ...JSON.parse(JSON.stringify(r)),
    list: { operation: r.list?.operation ?? "", params: { ...(r.list?.params ?? {}) } },
    get: { operation: r.get?.operation ?? "", params: { ...(r.get?.params ?? {}) } },
    pagination: { ...base.pagination, ...(r.pagination ?? {}) },
    fields: (r.fields ?? []).map((f) => ({
      key: f.key ?? "",
      pointer: f.pointer ?? "",
      label: f.label ?? "",
      type: f.type ?? "",
      remote: f.remote ?? "",
    })),
  };
}

watch(
  () => [props.open, props.resource] as const,
  ([open]) => {
    if (!open) return;
    draft.value = props.resource ? toDraft(props.resource) : blank();
  },
  { immediate: true },
);

/** Operation choices, labelled with method and path so an operationId like
 *  "get:/customers/{id}" is recognisable without cross-referencing the doc. */
const operationOptions = computed(() => {
  const ops = props.catalog?.operations ?? [];
  return ops.map((op) => ({
    value: op.id,
    label: `${op.method} ${op.path}${op.summary ? " — " + op.summary : ""}`.replace(" — ", " · "),
  }));
});

const listOptions = computed(() => operationOptions.value);
const getOptions = computed(() => [
  { value: "", label: t("apiclients.resource.none") },
  ...operationOptions.value,
]);

// An unset mode means an array, which is what nearly every document publishes.
// Naming it in the picker keeps the stored value empty while still showing the
// author which of the two shapes is in force.
const itemsModeOptions = computed(() => {
  const fallback = props.itemsModes[0] ?? "array";
  return [
    { value: "", label: t("apiclients.resource.items_mode_default", [fallback]) },
    ...props.itemsModes.map((m) => ({ value: m, label: t(ITEMS_MODE_LABEL_KEYS[m] ?? "") || m })),
  ];
});

// Explicit keys: the mode set is closed, and an interpolated lookup would hide
// these strings from every extractor.
const ITEMS_MODE_LABEL_KEYS: Record<string, string> = {
  array: "apiclients.resource.items_mode_array",
  map: "apiclients.resource.items_mode_map",
};

// A keyed collection carries its id in the key, so the pointer has nothing to
// address and the backend refuses it outright.
const keyedItems = computed(() => draft.value.items_mode === "map");

const keyStyleOptions = computed(() => [
  { value: "", label: t("apiclients.resource.none") },
  ...props.keyStyles.map((s) => ({ value: s, label: s })),
]);

const pagingOptions = computed(() =>
  props.paginationStyles.map((s) => ({ value: s, label: s })),
);

const pagingStyle = computed(() => draft.value.pagination.style || "none");
const usesOffset = computed(() => pagingStyle.value === "offset" || pagingStyle.value === "page");
const usesCursor = computed(() => pagingStyle.value === "cursor");
const usesLink = computed(() => pagingStyle.value === "link");

const canApply = computed(
  () => draft.value.key.trim().length > 0 && draft.value.list.operation.length > 0,
);

function addField() {
  draft.value.fields = [
    ...draft.value.fields,
    { key: "", pointer: "", label: "", type: "", remote: "" },
  ];
}

function removeField(index: number) {
  draft.value.fields = draft.value.fields.filter((_, i) => i !== index);
}

function apply() {
  if (!canApply.value) return;
  const out = JSON.parse(JSON.stringify(draft.value)) as DraftResource;
  if (out.items_mode === "map") out.id_path = "";
  // Drop half-typed field rows rather than saving a binding the backend would
  // reject for an empty key.
  out.fields = out.fields.filter((f) => f.key.trim().length > 0);
  emit("apply", out as unknown as Resource);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t(resource ? 'apiclients.resource.title_edit' : 'apiclients.resource.title_add')"
    width="720px"
    scroll
    @close="emit('close')"
  >
    <FormSection :title="t('apiclients.section.resources')">
      <FormRow :label="t('apiclients.resource.key')" :description="t('apiclients.resource.key_hint')">
        <TextField v-model="draft.key" />
      </FormRow>
      <FormRow :label="t('apiclients.resource.label')">
        <TextField v-model="draft.label" />
      </FormRow>
      <FormRow
        :label="t('apiclients.resource.list_op')"
        :description="t('apiclients.resource.list_op_hint')"
      >
        <SelectField v-model="draft.list.operation" :options="listOptions" />
      </FormRow>
      <FormRow
        :label="t('apiclients.resource.get_op')"
        :description="t('apiclients.resource.get_op_hint')"
      >
        <SelectField v-model="draft.get.operation" :options="getOptions" />
      </FormRow>
    </FormSection>

    <FormSection :title="t('apiclients.resource.id_path')">
      <FormRow
        :label="t('apiclients.resource.items_path')"
        :description="t('apiclients.resource.items_path_hint')"
      >
        <TextField v-model="draft.items_path" placeholder="/value" />
      </FormRow>
      <FormRow
        :label="t('apiclients.resource.items_mode')"
        :description="t('apiclients.resource.items_mode_hint')"
      >
        <SelectField v-model="draft.items_mode" :options="itemsModeOptions" />
      </FormRow>
      <FormRow
        v-if="!keyedItems"
        :label="t('apiclients.resource.id_path')"
      >
        <TextField v-model="draft.id_path" placeholder="/id" />
      </FormRow>
      <FormRow v-else :label="t('apiclients.resource.id_path')">
        <p class="muted small">{{ t('apiclients.resource.id_path_keyed') }}</p>
      </FormRow>
      <FormRow :label="t('apiclients.resource.label_path')">
        <TextField v-model="draft.label_path" placeholder="/name" />
      </FormRow>
      <FormRow :label="t('apiclients.resource.key_style')">
        <SelectField v-model="draft.key_style" :options="keyStyleOptions" />
      </FormRow>
    </FormSection>

    <FormSection :title="t('apiclients.resource.search_param')">
      <FormRow :label="t('apiclients.resource.search_param')">
        <TextField v-model="draft.search_param" placeholder="q" />
      </FormRow>
      <FormRow
        :label="t('apiclients.resource.search_template')"
        :description="t('apiclients.resource.search_template_hint')"
      >
        <TextField v-model="draft.search_template" />
      </FormRow>
      <FormRow
        :label="t('apiclients.resource.select_param')"
        :description="t('apiclients.resource.select_param_hint')"
      >
        <TextField v-model="draft.select_param" placeholder="$select" />
      </FormRow>
    </FormSection>

    <FormSection :title="t('apiclients.resource.pagination')">
      <FormRow :label="t('apiclients.resource.pagination')">
        <SelectField v-model="draft.pagination.style" :options="pagingOptions" />
      </FormRow>
      <FormRow v-if="usesOffset || usesCursor || usesLink" :label="t('apiclients.resource.limit_param')">
        <TextField v-model="draft.pagination.limit_param" placeholder="$top" />
      </FormRow>
      <FormRow v-if="usesOffset" :label="t('apiclients.resource.offset_param')">
        <TextField v-model="draft.pagination.offset_param" placeholder="$skip" />
      </FormRow>
      <FormRow v-if="usesCursor" :label="t('apiclients.resource.cursor_param')">
        <TextField v-model="draft.pagination.cursor_param" />
      </FormRow>
      <FormRow v-if="usesCursor" :label="t('apiclients.resource.cursor_path')">
        <TextField v-model="draft.pagination.cursor_path" />
      </FormRow>
      <FormRow v-if="usesLink" :label="t('apiclients.resource.link_path')">
        <TextField v-model="draft.pagination.link_path" placeholder="/@odata.nextLink" />
      </FormRow>
    </FormSection>

    <FormSection :title="t('apiclients.resource.fields')" :subtitle="t('apiclients.resource.fields_hint')">
      <table v-if="draft.fields.length > 0" class="apiclients-fields-table">
        <thead>
          <tr>
            <th>{{ t('apiclients.resource.field_key') }}</th>
            <th>{{ t('apiclients.resource.field_pointer') }}</th>
            <th>{{ t('apiclients.resource.field_remote') }}</th>
            <th class="apiclients-col-actions"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(f, i) in draft.fields" :key="i">
            <td><TextField v-model="f.key" /></td>
            <td><TextField v-model="f.pointer" placeholder="/Address/City" /></td>
            <td><TextField v-model="f.remote" /></td>
            <td class="apiclients-col-actions">
              <button
                class="tool-btn"
                type="button"
                :title="t('apiclients.resource.field_remove')"
                @click="removeField(i)"
              >
                <i class="fa-solid fa-trash" aria-hidden="true"></i>
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <button class="tool-btn" type="button" @click="addField">
        <i class="fa-solid fa-plus" aria-hidden="true"></i>
        {{ t('apiclients.resource.field_add') }}
      </button>
    </FormSection>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('close')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canApply" @click="apply">
        {{ t('apiclients.resource.apply') }}
      </button>
    </template>
  </Modal>
</template>
