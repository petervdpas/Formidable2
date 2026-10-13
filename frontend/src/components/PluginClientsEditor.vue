<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  Service as PluginSvc,
  type ShippedClient,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { useToast } from "../composables/useToast";
import { isValidPluginID } from "../composables/usePlugins";
import { usePluginEditor } from "../composables/usePluginEditor";
import { useAPIClients } from "../composables/useAPIClients";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useInformationSection } from "../composables/useInformationSection";
import { confirmLeave } from "../composables/useNavGuard";
import { backendErrMessage } from "../utils/backendError";
import CodeEditor from "./CodeEditor.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import { useConfirmAsk } from "../composables/useConfirmAsk";

// PluginClientsEditor - the API-clients tab of the plugin editor. This is
// where an api-client is named and where the OpenAPI/Swagger document it is
// generated from goes in: pasted or loaded from a file. Saving stores the
// document inside the plugin (api-clients/specs/<id>.<ext>) and lists the id
// under api_clients in plugin.json. Nothing is created by saving: the client
// is made when the user clicks the create button, or on another machine when
// the plugin zip (which carries the document) is imported.
//
// Backend owns the state: every action returns the fresh rows, and the
// manifest's api_clients is mirrored into the editor draft so a later Save
// of the manifest cannot drop it.

const props = defineProps<{ pluginId: string }>();

const { t } = useI18n();
const toast = useToast();
const { syncAPIClients } = usePluginEditor();
const { ask, request: askConfirm, confirm: confirmAsk, cancel: cancelAsk } = useConfirmAsk();
const apiClients = useAPIClients();
const { setActive: setWorkspace } = useActiveWorkspace();
const { setActive: setSection } = useInformationSection();

const rows = ref<ShippedClient[]>([]);
const activeID = ref<string>("");
const document = ref<string>("");
const baseline = ref<string>("");
// The base URL the plugin holds for the active client; empty means the
// document's own server.
const baseURL = ref<string>("");
const baselineURL = ref<string>("");
// The display name the plugin holds; empty means the document's title.
const clientName = ref<string>("");
const baselineName = ref<string>("");
const busy = ref<boolean>(false);

const showAdd = ref<boolean>(false);
const addInput = ref<string>("");
// An id typed in but not saved yet: it has no row until its document lands.
const pendingID = ref<string>("");

const fileInput = ref<HTMLInputElement | null>(null);

const activeRow = computed<ShippedClient | null>(
  () => rows.value.find((r) => r.id === activeID.value) ?? null,
);
const isPending = computed<boolean>(
  () => activeID.value !== "" && activeID.value === pendingID.value,
);
const docDirty = computed<boolean>(() => document.value !== baseline.value);
const settingsDirty = computed<boolean>(
  () =>
    baseURL.value.trim() !== baselineURL.value ||
    clientName.value.trim() !== baselineName.value,
);
const dirty = computed<boolean>(() => docDirty.value || settingsDirty.value);
// A document is JSON or YAML; the first character settles it, the same test
// the backend uses to pick the stored file's extension.
const documentLang = computed<"json" | "yaml">(() =>
  document.value.trimStart().startsWith("{") ? "json" : "yaml",
);
const addValid = computed<boolean>(() => {
  const id = addInput.value.trim();
  return isValidPluginID(id) && !rows.value.some((r) => r.id === id);
});

function adopt(next: ShippedClient[] | null): void {
  rows.value = next ?? [];
  syncAPIClients(rows.value.map((r) => r.id));
}

function adoptSettings(): void {
  baselineURL.value = activeRow.value?.base_url ?? "";
  baseURL.value = baselineURL.value;
  baselineName.value = activeRow.value?.name ?? "";
  clientName.value = baselineName.value;
}

async function loadDocument(): Promise<void> {
  document.value = "";
  baseline.value = "";
  adoptSettings();
  if (!props.pluginId || !activeID.value || isPending.value) return;
  try {
    const text = (await PluginSvc.GetShippedDocument(props.pluginId, activeID.value)) ?? "";
    document.value = text;
    baseline.value = text;
  } catch (err) {
    toast.error(backendErrMessage(err));
  }
}

async function load(): Promise<void> {
  pendingID.value = "";
  showAdd.value = false;
  addInput.value = "";
  if (!props.pluginId) {
    rows.value = [];
    activeID.value = "";
    return;
  }
  try {
    rows.value = (await PluginSvc.ListShippedClients(props.pluginId)) ?? [];
  } catch (err) {
    rows.value = [];
    toast.error(backendErrMessage(err));
  }
  activeID.value = rows.value[0]?.id ?? "";
  await loadDocument();
}

async function select(id: string): Promise<void> {
  if (id === activeID.value) return;
  activeID.value = id;
  await loadDocument();
}

function startClient(): void {
  const id = addInput.value.trim();
  if (!addValid.value) return;
  pendingID.value = id;
  activeID.value = id;
  document.value = "";
  baseline.value = "";
  baseURL.value = "";
  baselineURL.value = "";
  clientName.value = "";
  baselineName.value = "";
  showAdd.value = false;
  addInput.value = "";
}

function cancelAdd(): void {
  showAdd.value = false;
  addInput.value = "";
}

function pickFile(): void {
  fileInput.value?.click();
}

function onFile(e: Event): void {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => {
    document.value = typeof reader.result === "string" ? reader.result : "";
  };
  reader.onerror = () => toast.error("workspace.plugins.apiclients.file_error");
  reader.readAsText(file);
}

// One save for everything the plugin holds about the client. The document
// goes first: it is what declares a new client, and settings can only be
// stored for a declared one.
async function save(): Promise<void> {
  if (!props.pluginId || !activeID.value || !document.value.trim()) return;
  const id = activeID.value;
  busy.value = true;
  try {
    if (docDirty.value || pendingID.value === id) {
      adopt(await PluginSvc.SaveShippedClient(props.pluginId, id, document.value));
      if (pendingID.value === id) pendingID.value = "";
      baseline.value = document.value;
    }
    if (settingsDirty.value) {
      adopt(
        await PluginSvc.SetShippedSettings(
          props.pluginId,
          id,
          clientName.value.trim(),
          baseURL.value.trim(),
        ),
      );
      adoptSettings();
    }
    toast.success("workspace.plugins.apiclients.save_success", [id]);
  } catch (err) {
    toast.error("workspace.plugins.apiclients.save_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

// The client is created from the document as it is stored in the plugin, so
// the button waits for unsaved edits to be saved first.
const canGenerate = computed<boolean>(
  () => !!activeRow.value?.document && !activeRow.value.installed && !dirty.value,
);

async function generate(): Promise<void> {
  if (!props.pluginId || !activeID.value) return;
  const id = activeID.value;
  busy.value = true;
  try {
    adopt(await PluginSvc.GenerateShippedClient(props.pluginId, id));
  } catch (err) {
    toast.error("workspace.plugins.apiclients.generate_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

// Jump to Information > API clients with this client selected.
async function openClient(): Promise<void> {
  const id = activeID.value;
  if (!id || !(await confirmLeave())) return;
  await apiClients.refresh();
  await apiClients.select(id);
  setWorkspace("information");
  setSection("api-clients");
}

async function remove(): Promise<void> {
  if (!props.pluginId || !activeID.value) return;
  const id = activeID.value;
  if (isPending.value) {
    pendingID.value = "";
    activeID.value = rows.value[0]?.id ?? "";
    await loadDocument();
    return;
  }
  askConfirm({
    title: t("workspace.plugins.apiclients.remove_title"),
    message: t("workspace.plugins.apiclients.remove_confirm", [id]),
    confirmLabel: t("workspace.plugins.apiclients.remove"),
    run: () => doRemove(id),
  });
}

async function doRemove(id: string): Promise<void> {
  busy.value = true;
  try {
    adopt(await PluginSvc.RemoveShippedClient(props.pluginId, id));
    activeID.value = rows.value[0]?.id ?? "";
    await loadDocument();
  } catch (err) {
    toast.error(backendErrMessage(err));
  } finally {
    busy.value = false;
  }
}

watch(() => props.pluginId, () => void load(), { immediate: true });
</script>

<template>
  <!-- plugin-i18n-editor supplies the shared chip-switcher and action-row styles. -->
  <div class="plugin-i18n-editor plugin-clients-editor">
    <p class="muted small i18n-help">
      {{ t("workspace.plugins.apiclients.help") }}
    </p>

    <div class="locale-switcher">
      <button
        v-for="row in rows"
        :key="row.id"
        type="button"
        :class="['locale-chip', { active: row.id === activeID }]"
        @click="select(row.id)"
      >
        {{ row.id }}
      </button>
      <button
        v-if="pendingID"
        type="button"
        class="locale-chip locale-chip-add active"
      >
        {{ pendingID }}
      </button>
      <button
        v-if="!showAdd"
        type="button"
        class="locale-chip locale-chip-add"
        :disabled="pendingID !== ''"
        @click="showAdd = true"
      >
        {{ t("workspace.plugins.apiclients.add") }}
      </button>
      <template v-else>
        <input
          v-model="addInput"
          class="locale-add-input mono"
          :placeholder="t('workspace.plugins.apiclients.add_placeholder')"
          @keydown.enter.prevent="startClient"
        />
        <button type="button" class="tool-btn small" :disabled="!addValid" @click="startClient">+</button>
        <button type="button" class="tool-btn small" @click="cancelAdd">×</button>
      </template>
    </div>

    <p v-if="rows.length === 0 && !pendingID" class="muted small">
      {{ t("workspace.plugins.apiclients.empty") }}
    </p>

    <template v-else-if="activeID">
      <dl class="client-facts">
        <div>
          <dt class="meta-key small">{{ t("workspace.plugins.apiclients.fact_client") }}</dt>
          <dd><code class="meta-value mono">{{ activeID }}</code></dd>
        </div>
        <div v-if="activeRow?.document">
          <dt class="meta-key small">{{ t("workspace.plugins.apiclients.fact_document") }}</dt>
          <dd>
            <code class="meta-value mono">api-clients/specs/{{ activeRow.document }}</code>
            <span v-if="activeRow.title" class="muted small">
              {{ t("workspace.plugins.apiclients.fact_title", [activeRow.title, activeRow.operations]) }}
            </span>
          </dd>
        </div>
        <div>
          <dt class="meta-key small">{{ t("workspace.plugins.apiclients.fact_state") }}</dt>
          <dd class="small">
            <template v-if="isPending">{{ t("workspace.plugins.apiclients.state_pending") }}</template>
            <template v-else-if="activeRow?.installed">{{ t("workspace.plugins.apiclients.state_installed") }}</template>
            <template v-else-if="activeRow?.document">{{ t("workspace.plugins.apiclients.state_not_installed") }}</template>
            <template v-else>{{ t("workspace.plugins.apiclients.state_no_document") }}</template>
          </dd>
        </div>
      </dl>

      <div class="client-url">
        <label class="meta-key small" for="plugin-client-name">
          {{ t("workspace.plugins.apiclients.name") }}
        </label>
        <input
          id="plugin-client-name"
          v-model="clientName"
          class="field-input"
          :placeholder="activeRow?.title || activeID"
        />
        <p class="muted small">
          {{ t("workspace.plugins.apiclients.name_help", [activeRow?.title || activeID]) }}
        </p>
      </div>

      <div class="client-url">
        <label class="meta-key small" for="plugin-client-base-url">
          {{ t("workspace.plugins.apiclients.base_url") }}
        </label>
        <input
          id="plugin-client-base-url"
          v-model="baseURL"
          class="field-input mono"
          spellcheck="false"
          :placeholder="activeRow?.server || 'https://'"
        />
        <p class="muted small">
          <template v-if="activeRow?.server">
            {{ t("workspace.plugins.apiclients.base_url_help_server", [activeRow.server]) }}
          </template>
          <template v-else>
            {{ t("workspace.plugins.apiclients.base_url_help_none") }}
          </template>
        </p>
      </div>

      <p v-if="activeRow?.error" class="client-error small">{{ activeRow.error }}</p>
      <p v-if="activeRow?.has_definition" class="muted small">
        {{ t("workspace.plugins.apiclients.has_definition") }}
      </p>

      <p v-if="!document.trim()" class="muted small">
        {{ t("workspace.plugins.apiclients.document_placeholder") }}
      </p>
      <CodeEditor
        v-model="document"
        :lang="documentLang"
        :height="420"
        :title="`${activeID} • ${t('workspace.plugins.apiclients.fact_document')}`"
      />
      <input
        ref="fileInput"
        type="file"
        accept=".json,.yaml,.yml,application/json"
        hidden
        @change="onFile"
      />

      <div class="i18n-actions">
        <button type="button" class="tool-btn" @click="pickFile">
          {{ t("workspace.plugins.apiclients.load_file") }}
        </button>
        <button
          type="button"
          class="tool-btn primary"
          :disabled="busy || !dirty || !document.trim()"
          @click="save"
        >
          {{ t("workspace.plugins.apiclients.save") }}
        </button>
        <button
          v-if="activeRow?.installed"
          type="button"
          class="tool-btn"
          @click="openClient"
        >
          {{ t("workspace.plugins.apiclients.open") }}
        </button>
        <button
          v-else
          type="button"
          class="tool-btn primary"
          :disabled="busy || !canGenerate"
          @click="generate"
        >
          {{ t("workspace.plugins.apiclients.generate") }}
        </button>
        <button type="button" class="tool-btn danger" :disabled="busy" @click="remove">
          {{ t("workspace.plugins.apiclients.remove") }}
        </button>
      </div>
    </template>

    <ConfirmDialog
      :open="ask !== null"
      :title="ask?.title"
      :message="ask?.message"
      :confirm-label="ask?.confirmLabel"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @confirm="confirmAsk"
      @cancel="cancelAsk"
    />
  </div>
</template>
