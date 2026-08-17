<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useAPIClients } from "../../composables/useAPIClients";
import { useVault } from "../../composables/useVault";
import { useToast } from "../../composables/useToast";
import ConfirmDialog from "../../components/ConfirmDialog.vue";
import Modal from "../../components/Modal.vue";
import { FormRow, FormSection, SelectField, TextField } from "../../components/fields";
import APIClientResourceDialog from "./APIClientResourceDialog.vue";
import type {
  Connection,
  Resource,
  ValidationError,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/connection";

const { t } = useI18n();
const toast = useToast();

const {
  summaries,
  detail,
  loading,
  lastError,
  dialects,
  keyStyles,
  paginationStyles,
  hasClients,
  selectedId,
  refresh,
  select,
  importSpec,
  save,
  remove,
  validate,
  reloadSpecs,
  listItems,
  setCredential,
  forgetCredential,
} = useAPIClients();

const { unlocked: vaultUnlocked, refresh: refreshVault } = useVault();

// The edited copy. Saving a client means sending the whole document back, so
// the editor works on a clone and the list keeps showing what is on disk.
const draft = ref<Connection | null>(null);
const problems = ref<ValidationError[]>([]);
const busy = ref(false);

const newOpen = ref(false);
const newId = ref("");
const newName = ref("");
const newSpecFile = ref("");
const newSpecOps = ref(0);
const newSpecData = ref("");
const specInput = ref<HTMLInputElement | null>(null);

const resourceOpen = ref(false);
const resourceIndex = ref(-1);

const credential = ref("");
const deleteTarget = ref("");
const deleteOpen = computed(() => deleteTarget.value !== "");

const testSearch = ref("");
const testRunning = ref(false);
const testRows = ref<{ id: string; label: string; fields: Record<string, string> }[] | null>(null);
const testResource = ref("");

const dialectOptions = computed(() => dialects.value.map((d) => ({ value: d, label: d })));
const authKindOptions = computed(() =>
  ["none", "bearer", "apikey", "basic"].map((k) => ({ value: k, label: k })),
);
const authInOptions = computed(() =>
  ["header", "query"].map((k) => ({ value: k, label: k })),
);

const authKind = computed(() => draft.value?.auth?.kind || "none");
const usesAPIKey = computed(() => authKind.value === "apikey");
const usesBasic = computed(() => authKind.value === "basic");
const needsCredential = computed(() => authKind.value !== "none" && authKind.value !== "");

const canCreate = computed(
  () => !busy.value && newId.value.trim().length > 0 && newSpecData.value.length > 0,
);

const specSummary = computed(() => {
  const cat = detail.value?.catalog;
  if (!cat) return "";
  return t("apiclients.spec.summary", [
    cat.title || "",
    cat.version || "",
    String(cat.operations?.length ?? 0),
  ]);
});

onMounted(() => {
  void refresh();
  void refreshVault();
});

// Re-clone whenever a different client is selected, so the editor never shows
// one client's edits under another's name.
watch(detail, (d) => {
  draft.value = d ? (JSON.parse(JSON.stringify(d.client)) as Connection) : null;
  credential.value = "";
  testRows.value = null;
  testResource.value = d?.client.resources?.[0]?.key ?? "";
  void revalidate();
});

async function revalidate() {
  problems.value = draft.value ? await validate(draft.value) : [];
}

async function onPick(id: string) {
  const r = await select(id);
  if (!r.ok) toast.error("apiclients.toast.load_failed", [r.message]);
}

// New client ------------------------------------------------------------

function openNew() {
  newId.value = "";
  newName.value = "";
  newSpecFile.value = "";
  newSpecOps.value = 0;
  newSpecData.value = "";
  newOpen.value = true;
}

function pickSpec() {
  specInput.value?.click();
}

async function onSpecPicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;

  const base64 = await readAsBase64(file);
  // Import under the id so the stored document lands beside the definition.
  // Parsing happens in Go, so an unreadable document fails here, not later.
  const name = (newId.value.trim() || file.name.replace(/\.[^.]+$/, "")).toLowerCase();
  const r = await importSpec(name, base64);
  if (!r.ok) {
    toast.error("apiclients.toast.spec_failed", [r.message]);
    return;
  }
  if (!newId.value.trim()) newId.value = name;
  if (!newName.value.trim()) newName.value = r.catalog?.title ?? name;
  newSpecFile.value = r.file;
  newSpecOps.value = r.catalog?.operations?.length ?? 0;
  newSpecData.value = base64;
}

async function createClient() {
  if (!canCreate.value) return;
  const id = newId.value.trim();
  const client = {
    id,
    name: newName.value.trim() || id,
    spec_file: newSpecFile.value,
    auth: { kind: "none" },
    resources: [],
  } as unknown as Connection;

  busy.value = true;
  const r = await save(client);
  busy.value = false;
  if (r.ok) {
    newOpen.value = false;
    toast.success("apiclients.toast.created", [id]);
  } else {
    toast.error("apiclients.toast.create_failed", [r.message]);
  }
}

// Editing ---------------------------------------------------------------

async function onSave() {
  if (!draft.value) return;
  busy.value = true;
  const r = await save(draft.value);
  busy.value = false;
  if (r.ok) toast.success("apiclients.toast.saved", [draft.value.id]);
  else toast.error("apiclients.toast.save_failed", [r.message]);
}

async function confirmDelete() {
  const id = deleteTarget.value;
  deleteTarget.value = "";
  const r = await remove(id);
  if (r.ok) toast.success("apiclients.toast.deleted", [id]);
  else toast.error("apiclients.toast.delete_failed", [r.message]);
}

async function onReloadSpecs() {
  const r = await reloadSpecs();
  if (r.ok) toast.success("apiclients.toast.specs_reloaded");
  else toast.error("apiclients.toast.load_failed", [r.message]);
}

// Resources -------------------------------------------------------------

function addResource() {
  resourceIndex.value = -1;
  resourceOpen.value = true;
}

function editResource(index: number) {
  resourceIndex.value = index;
  resourceOpen.value = true;
}

function removeResource(index: number) {
  if (!draft.value) return;
  draft.value.resources = (draft.value.resources ?? []).filter((_, i) => i !== index);
  void revalidate();
}

function applyResource(value: Resource) {
  if (!draft.value) return;
  const list = [...(draft.value.resources ?? [])];
  if (resourceIndex.value < 0) list.push(value);
  else list[resourceIndex.value] = value;
  draft.value.resources = list;
  resourceOpen.value = false;
  void revalidate();
}

const editingResource = computed<Resource | null>(() =>
  resourceIndex.value >= 0 ? (draft.value?.resources?.[resourceIndex.value] ?? null) : null,
);

// Credential ------------------------------------------------------------

async function onSaveCredential() {
  if (!draft.value || !credential.value) return;
  const r = await setCredential(draft.value.id, credential.value);
  credential.value = "";
  if (r.ok) toast.success("apiclients.toast.credential_saved");
  else toast.error("apiclients.toast.credential_failed", [r.message]);
}

async function onForgetCredential() {
  if (!draft.value) return;
  const r = await forgetCredential(draft.value.id);
  if (r.ok) toast.success("apiclients.toast.credential_forgotten");
  else toast.error("apiclients.toast.credential_failed", [r.message]);
}

// Try it ----------------------------------------------------------------

async function runTest() {
  if (!draft.value || !testResource.value) return;
  testRunning.value = true;
  testRows.value = null;
  const r = await listItems({
    connection: draft.value.id,
    resource: testResource.value,
    search: testSearch.value,
    limit: 10,
  } as never);
  testRunning.value = false;
  if (!r.ok) {
    toast.error("apiclients.toast.test_failed", [r.message]);
    return;
  }
  testRows.value = (r.page?.items ?? []).map((i) => ({
    id: i.id,
    label: i.label,
    fields: (i.fields ?? {}) as Record<string, string>,
  }));
}

const resourceOptions = computed(() =>
  (draft.value?.resources ?? []).map((r) => ({ value: r.key, label: r.label || r.key })),
);

function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}
</script>

<template>
  <p class="section-info">{{ t('apiclients.info') }}</p>

  <p v-if="lastError" class="apiclients-error">
    <i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>
    {{ t('apiclients.toast.load_failed', [lastError]) }}
  </p>

  <div class="apiclients-layout">
    <aside class="apiclients-list">
      <div class="apiclients-list-header">
        <h4>{{ t('apiclients.list.title') }}</h4>
        <div class="apiclients-list-actions">
          <button
            class="tool-btn"
            type="button"
            :disabled="loading"
            :title="t('apiclients.action.reload_specs')"
            :aria-label="t('apiclients.action.reload_specs')"
            @click="onReloadSpecs"
          >
            <i class="fa-solid fa-rotate" aria-hidden="true"></i>
          </button>
          <button class="tool-btn primary" type="button" @click="openNew">
            <i class="fa-solid fa-plus" aria-hidden="true"></i>
            {{ t('apiclients.action.new') }}
          </button>
        </div>
      </div>

      <p v-if="loading" class="muted small">{{ t('apiclients.list.loading') }}</p>
      <p v-else-if="!hasClients" class="muted small">{{ t('apiclients.list.empty') }}</p>

      <ul v-else class="apiclients-rows">
        <li
          v-for="s in summaries"
          :key="s.id"
          :class="['apiclients-row', { active: s.id === selectedId }]"
          @click="onPick(s.id)"
        >
          <span class="apiclients-row-name">{{ s.name || s.id }}</span>
          <code class="apiclients-row-id">{{ s.id }}</code>
          <span v-if="!s.ok" class="apiclients-row-warn" :title="s.error">
            <i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i>
            {{ t('apiclients.list.broken') }}
          </span>
        </li>
      </ul>
    </aside>

    <section class="apiclients-editor">
      <p v-if="!draft" class="muted">{{ t('apiclients.editor.idle') }}</p>

      <template v-else>
        <div class="apiclients-editor-header">
          <h4>{{ draft.name || draft.id }}</h4>
          <span class="apiclients-spacer"></span>
          <button class="tool-btn" type="button" @click="deleteTarget = draft.id">
            <i class="fa-solid fa-trash" aria-hidden="true"></i>
            {{ t('apiclients.action.delete') }}
          </button>
          <button class="tool-btn primary" type="button" :disabled="busy" @click="onSave">
            <i class="fa-solid fa-floppy-disk" aria-hidden="true"></i>
            {{ t('apiclients.action.save') }}
          </button>
        </div>

        <p v-if="problems.length === 0" class="apiclients-clean">
          <i class="fa-solid fa-circle-check" aria-hidden="true"></i>
          {{ t('apiclients.validation.clean') }}
        </p>
        <div v-else class="apiclients-problems">
          <strong>{{ t('apiclients.validation.title') }}</strong>
          <ul>
            <li v-for="(p, i) in problems" :key="i">
              <code v-if="p.resource">{{ p.resource }}</code>
              {{ p.message }}
            </li>
          </ul>
        </div>

        <FormSection :title="t('apiclients.section.identity')" :subtitle="specSummary">
          <FormRow :label="t('apiclients.field.name')">
            <TextField v-model="draft.name" @update:model-value="revalidate" />
          </FormRow>
          <FormRow
            :label="t('apiclients.field.base_url')"
            :description="t('apiclients.field.base_url_hint')"
          >
            <TextField v-model="draft.base_url" @update:model-value="revalidate" />
          </FormRow>
          <FormRow
            :label="t('apiclients.field.spec_url')"
            :description="t('apiclients.field.spec_url_hint')"
          >
            <TextField v-model="draft.spec_url" />
          </FormRow>
          <FormRow
            :label="t('apiclients.field.dialect')"
            :description="t('apiclients.field.dialect_hint')"
          >
            <SelectField
              v-model="draft.dialect"
              :options="dialectOptions"
              @update:model-value="revalidate"
            />
          </FormRow>
          <FormRow :label="t('apiclients.field.spec')">
            <code class="apiclients-specfile">{{ draft.spec_file }}</code>
          </FormRow>
        </FormSection>

        <FormSection :title="t('apiclients.section.auth')">
          <FormRow :label="t('apiclients.auth.kind')">
            <SelectField
              v-model="draft.auth.kind"
              :options="authKindOptions"
              @update:model-value="revalidate"
            />
          </FormRow>
          <FormRow v-if="usesAPIKey" :label="t('apiclients.auth.in')">
            <SelectField
              v-model="draft.auth.in"
              :options="authInOptions"
              @update:model-value="revalidate"
            />
          </FormRow>
          <FormRow v-if="usesAPIKey" :label="t('apiclients.auth.param')">
            <TextField v-model="draft.auth.name" @update:model-value="revalidate" />
          </FormRow>
          <FormRow v-if="usesBasic" :label="t('apiclients.auth.user')">
            <TextField v-model="draft.auth.user" @update:model-value="revalidate" />
          </FormRow>

          <FormRow
            v-if="needsCredential"
            :label="t('apiclients.auth.credential')"
            :description="t('apiclients.auth.credential_hint')"
          >
            <p v-if="!vaultUnlocked" class="muted small">
              {{ t('apiclients.auth.credential_locked') }}
            </p>
            <template v-else>
              <p class="muted small">
                {{
                  detail?.has_credential
                    ? t('apiclients.auth.credential_stored')
                    : t('apiclients.auth.credential_none')
                }}
              </p>
              <TextField v-model="credential" type="password" autocomplete="off" />
              <div class="apiclients-actions">
                <button
                  class="tool-btn"
                  type="button"
                  :disabled="!credential"
                  @click="onSaveCredential"
                >
                  {{ t('apiclients.auth.credential_save') }}
                </button>
                <button
                  v-if="detail?.has_credential"
                  class="tool-btn"
                  type="button"
                  @click="onForgetCredential"
                >
                  {{ t('apiclients.auth.credential_forget') }}
                </button>
              </div>
            </template>
          </FormRow>
        </FormSection>

        <FormSection :title="t('apiclients.section.resources')">
          <p v-if="(draft.resources ?? []).length === 0" class="muted small">
            {{ t('apiclients.resource.empty') }}
          </p>
          <table v-else class="apiclients-resources-table">
            <tbody>
              <tr v-for="(r, i) in draft.resources" :key="r.key">
                <td>
                  <code>{{ r.key }}</code>
                  <span v-if="r.label" class="muted small"> {{ r.label }}</span>
                </td>
                <td class="muted small">
                  {{ r.list?.operation }}<span v-if="r.get?.operation"> · {{ r.get.operation }}</span>
                </td>
                <td class="muted small">{{ (r.fields ?? []).length }}</td>
                <td class="apiclients-col-actions">
                  <button
                    class="tool-btn"
                    type="button"
                    :title="t('apiclients.resource.edit')"
                    :aria-label="t('apiclients.resource.edit')"
                    @click="editResource(i)"
                  >
                    <i class="fa-solid fa-pen" aria-hidden="true"></i>
                  </button>
                  <button
                    class="tool-btn"
                    type="button"
                    :title="t('apiclients.resource.remove')"
                    :aria-label="t('apiclients.resource.remove')"
                    @click="removeResource(i)"
                  >
                    <i class="fa-solid fa-trash" aria-hidden="true"></i>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>

          <button class="tool-btn" type="button" @click="addResource">
            <i class="fa-solid fa-plus" aria-hidden="true"></i>
            {{ t('apiclients.resource.add') }}
          </button>
        </FormSection>

        <FormSection
          v-if="(draft.resources ?? []).length > 0"
          :title="t('apiclients.test.title')"
          :subtitle="t('apiclients.test.hint')"
        >
          <FormRow :label="t('apiclients.section.resources')">
            <SelectField v-model="testResource" :options="resourceOptions" />
          </FormRow>
          <FormRow :label="t('apiclients.test.search')">
            <TextField v-model="testSearch" @keydown.enter="runTest" />
          </FormRow>
          <FormRow>
            <button class="tool-btn primary" type="button" :disabled="testRunning" @click="runTest">
              <i class="fa-solid fa-play" aria-hidden="true"></i>
              {{ testRunning ? t('apiclients.test.running') : t('apiclients.test.run') }}
            </button>
          </FormRow>

          <template v-if="testRows">
            <p v-if="testRows.length === 0" class="muted small">{{ t('apiclients.test.empty') }}</p>
            <template v-else>
              <p class="muted small">{{ t('apiclients.test.count', [String(testRows.length)]) }}</p>
              <table class="apiclients-test-table">
                <tbody>
                  <tr v-for="(row, i) in testRows" :key="i">
                    <td><code>{{ row.id }}</code></td>
                    <td>{{ row.label }}</td>
                    <td class="muted small">
                      <span v-for="(v, k) in row.fields" :key="k" class="apiclients-chip">
                        {{ k }}={{ v }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </template>
          </template>
        </FormSection>
      </template>
    </section>
  </div>

  <Modal :open="newOpen" :title="t('apiclients.new.title')" width="560px" @close="newOpen = false">
    <FormRow :label="t('apiclients.new.id')" :description="t('apiclients.new.id_hint')">
      <TextField v-model="newId" :disabled="busy" />
    </FormRow>
    <FormRow :label="t('apiclients.new.name')">
      <TextField v-model="newName" :disabled="busy" />
    </FormRow>
    <FormRow :label="t('apiclients.new.spec')">
      <button class="tool-btn" type="button" @click="pickSpec">
        <i class="fa-solid fa-upload" aria-hidden="true"></i>
        {{ t('apiclients.new.spec_pick') }}
      </button>
      <input
        ref="specInput"
        type="file"
        accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml"
        class="apiclients-file-input"
        @change="onSpecPicked"
      />
      <p v-if="newSpecFile" class="muted small">
        {{ t('apiclients.new.spec_chosen', [newSpecFile, String(newSpecOps)]) }}
      </p>
    </FormRow>

    <template #footer>
      <button class="tool-btn" type="button" @click="newOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" :disabled="!canCreate" @click="createClient">
        {{ t('apiclients.new.create') }}
      </button>
    </template>
  </Modal>

  <APIClientResourceDialog
    :open="resourceOpen"
    :resource="editingResource"
    :catalog="detail?.catalog ?? null"
    :key-styles="keyStyles"
    :pagination-styles="paginationStyles"
    @apply="applyResource"
    @close="resourceOpen = false"
  />

  <ConfirmDialog
    :open="deleteOpen"
    :title="t('apiclients.confirm.delete_title')"
    :message="t('apiclients.confirm.delete_message', [deleteTarget])"
    :confirm-label="t('apiclients.action.delete')"
    :cancel-label="t('common.cancel')"
    variant="danger"
    @cancel="deleteTarget = ''"
    @confirm="confirmDelete"
  />
</template>
