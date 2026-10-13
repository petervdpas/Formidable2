<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  Service as PluginSvc,
  type ShippedTemplate,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/plugin";
import { useToast } from "../composables/useToast";
import { useTemplates } from "../composables/useTemplates";
import { backendErrMessage } from "../utils/backendError";
import CodeEditor from "./CodeEditor.vue";
import ConfirmDialog from "./ConfirmDialog.vue";
import { useConfirmAsk } from "../composables/useConfirmAsk";

// PluginTemplatesEditor - the "Shipped templates" tab of the plugin editor.
// A plugin can hold templates of its own (plugins/<id>/templates/<file>.yaml)
// and create them in the open context folder, so its script has somewhere to
// store what it fetches. A template is taken over from the Templates workspace
// or typed here as YAML. Saving only stores the copy in the plugin; the
// template itself is made when the user clicks the create button, or when the
// plugin zip is imported. A template that already exists is never replaced.
//
// Backend owns the state: every action returns the fresh rows.

const props = defineProps<{ pluginId: string }>();
// Fired whenever the set of held or created templates changes, so the
// manifest's template binding list can follow.
const emit = defineEmits<{ changed: [] }>();

const { t } = useI18n();
const toast = useToast();
const { filenames, refresh: refreshTemplates } = useTemplates();
const { ask, request: askConfirm, confirm: confirmAsk, cancel: cancelAsk } = useConfirmAsk();

const rows = ref<ShippedTemplate[]>([]);
const activeFile = ref<string>("");
const yaml = ref<string>("");
const baseline = ref<string>("");
const busy = ref<boolean>(false);

const showAdd = ref<boolean>(false);
const pickExisting = ref<string>("");
const newName = ref<string>("");
// A file name typed in but not saved yet: it has no row until its YAML lands.
const pendingFile = ref<string>("");

const activeRow = computed<ShippedTemplate | null>(
  () => rows.value.find((r) => r.file === activeFile.value) ?? null,
);
const isPending = computed<boolean>(
  () => activeFile.value !== "" && activeFile.value === pendingFile.value,
);
const dirty = computed<boolean>(() => yaml.value !== baseline.value);
// Templates of the context folder the plugin does not hold yet.
const takeable = computed<string[]>(() =>
  filenames.value.filter((f) => !rows.value.some((r) => r.file === f)),
);
// Same rule as the backend: a plain "<name>.yaml".
const newFile = computed<string>(() => {
  const raw = newName.value.trim();
  if (!raw) return "";
  return raw.endsWith(".yaml") ? raw : `${raw}.yaml`;
});
const newValid = computed<boolean>(
  () =>
    /^[^./\\\s][^/\\]*\.yaml$/.test(newFile.value) &&
    !newFile.value.includes("..") &&
    !rows.value.some((r) => r.file === newFile.value),
);
const canCreate = computed<boolean>(
  () => !!activeRow.value && !activeRow.value.installed && !activeRow.value.error && !dirty.value,
);

async function loadYAML(): Promise<void> {
  yaml.value = "";
  baseline.value = "";
  if (!props.pluginId || !activeFile.value || isPending.value) return;
  try {
    const text = (await PluginSvc.GetShippedTemplate(props.pluginId, activeFile.value)) ?? "";
    yaml.value = text;
    baseline.value = text;
  } catch (err) {
    toast.error(backendErrMessage(err));
  }
}

function closeAdd(): void {
  showAdd.value = false;
  pickExisting.value = "";
  newName.value = "";
}

async function load(): Promise<void> {
  pendingFile.value = "";
  closeAdd();
  if (!props.pluginId) {
    rows.value = [];
    activeFile.value = "";
    return;
  }
  try {
    rows.value = (await PluginSvc.ListShippedTemplates(props.pluginId)) ?? [];
  } catch (err) {
    rows.value = [];
    toast.error(backendErrMessage(err));
  }
  activeFile.value = rows.value[0]?.file ?? "";
  await loadYAML();
}

async function select(file: string): Promise<void> {
  if (file === activeFile.value) return;
  activeFile.value = file;
  await loadYAML();
}

// Copy a template from the context folder into the plugin. Also used to
// refresh the plugin's copy from the template as it is now.
async function takeOver(file: string): Promise<void> {
  if (!props.pluginId || !file) return;
  busy.value = true;
  try {
    rows.value = (await PluginSvc.AddShippedTemplateFrom(props.pluginId, file)) ?? [];
    emit("changed");
    closeAdd();
    pendingFile.value = "";
    activeFile.value = file;
    await loadYAML();
    toast.success("workspace.plugins.heldtemplates.save_success", [file]);
  } catch (err) {
    toast.error("workspace.plugins.heldtemplates.save_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

function startNew(): void {
  if (!newValid.value) return;
  pendingFile.value = newFile.value;
  activeFile.value = newFile.value;
  yaml.value = "";
  baseline.value = "";
  closeAdd();
}

async function save(): Promise<void> {
  if (!props.pluginId || !activeFile.value || !yaml.value.trim()) return;
  const file = activeFile.value;
  busy.value = true;
  try {
    rows.value = (await PluginSvc.SaveShippedTemplate(props.pluginId, file, yaml.value)) ?? [];
    emit("changed");
    if (pendingFile.value === file) pendingFile.value = "";
    baseline.value = yaml.value;
    toast.success("workspace.plugins.heldtemplates.save_success", [file]);
  } catch (err) {
    toast.error("workspace.plugins.heldtemplates.save_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

async function create(): Promise<void> {
  if (!props.pluginId || !activeFile.value) return;
  const file = activeFile.value;
  busy.value = true;
  try {
    rows.value = (await PluginSvc.CreateShippedTemplate(props.pluginId, file)) ?? [];
    emit("changed");
    await refreshTemplates();
    toast.success("workspace.plugins.heldtemplates.create_success", [file]);
  } catch (err) {
    toast.error("workspace.plugins.heldtemplates.create_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

// The one deliberate way a template is overwritten: the user asks for the
// plugin's version. The button waits for unsaved edits, since the copy stored
// in the plugin is what gets written.
const canReplace = computed<boolean>(
  () => !!activeRow.value?.installed && activeRow.value.differs && !activeRow.value.error && !dirty.value,
);

function replace(): void {
  if (!props.pluginId || !activeFile.value) return;
  const file = activeFile.value;
  askConfirm({
    title: t("workspace.plugins.heldtemplates.replace_title"),
    message: t("workspace.plugins.heldtemplates.replace_confirm", [file]),
    confirmLabel: t("workspace.plugins.heldtemplates.replace"),
    run: () => doReplace(file),
  });
}

async function doReplace(file: string): Promise<void> {
  busy.value = true;
  try {
    rows.value = (await PluginSvc.ReplaceShippedTemplate(props.pluginId, file)) ?? [];
    emit("changed");
    await refreshTemplates();
    toast.success("workspace.plugins.heldtemplates.replace_success", [file]);
  } catch (err) {
    toast.error("workspace.plugins.heldtemplates.replace_error", [backendErrMessage(err)]);
  } finally {
    busy.value = false;
  }
}

async function remove(): Promise<void> {
  if (!props.pluginId || !activeFile.value) return;
  const file = activeFile.value;
  if (isPending.value) {
    pendingFile.value = "";
    activeFile.value = rows.value[0]?.file ?? "";
    await loadYAML();
    return;
  }
  askConfirm({
    title: t("workspace.plugins.heldtemplates.remove_title"),
    message: t("workspace.plugins.heldtemplates.remove_confirm", [file]),
    confirmLabel: t("workspace.plugins.heldtemplates.remove"),
    run: () => doRemove(file),
  });
}

async function doRemove(file: string): Promise<void> {
  busy.value = true;
  try {
    rows.value = (await PluginSvc.RemoveShippedTemplate(props.pluginId, file)) ?? [];
    emit("changed");
    activeFile.value = rows.value[0]?.file ?? "";
    await loadYAML();
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
      {{ t("workspace.plugins.heldtemplates.help") }}
    </p>

    <div class="locale-switcher">
      <button
        v-for="row in rows"
        :key="row.file"
        type="button"
        :class="['locale-chip', { active: row.file === activeFile }]"
        @click="select(row.file)"
      >
        {{ row.file }}
      </button>
      <button v-if="pendingFile" type="button" class="locale-chip locale-chip-add active">
        {{ pendingFile }}
      </button>
      <button
        v-if="!showAdd"
        type="button"
        class="locale-chip locale-chip-add"
        :disabled="pendingFile !== ''"
        @click="showAdd = true"
      >
        {{ t("workspace.plugins.heldtemplates.add") }}
      </button>
    </div>

    <div v-if="showAdd" class="client-url">
      <label class="meta-key small" for="plugin-template-take">
        {{ t("workspace.plugins.heldtemplates.take_label") }}
      </label>
      <div class="locale-switcher">
        <select id="plugin-template-take" v-model="pickExisting" class="locale-add-select">
          <option value="">{{ t("workspace.plugins.heldtemplates.take_placeholder") }}</option>
          <option v-for="f in takeable" :key="f" :value="f">{{ f }}</option>
        </select>
        <button
          type="button"
          class="tool-btn small"
          :disabled="busy || !pickExisting"
          @click="takeOver(pickExisting)"
        >
          {{ t("workspace.plugins.heldtemplates.take") }}
        </button>
      </div>
      <label class="meta-key small" for="plugin-template-new">
        {{ t("workspace.plugins.heldtemplates.new_label") }}
      </label>
      <div class="locale-switcher">
        <input
          id="plugin-template-new"
          v-model="newName"
          class="locale-add-input mono"
          :placeholder="t('workspace.plugins.heldtemplates.new_placeholder')"
          @keydown.enter.prevent="startNew"
        />
        <button type="button" class="tool-btn small" :disabled="!newValid" @click="startNew">+</button>
        <button type="button" class="tool-btn small" @click="closeAdd">×</button>
      </div>
    </div>

    <p v-if="rows.length === 0 && !pendingFile" class="muted small">
      {{ t("workspace.plugins.heldtemplates.empty") }}
    </p>

    <template v-else-if="activeFile">
      <dl class="client-facts">
        <div>
          <dt class="meta-key small">{{ t("workspace.plugins.heldtemplates.fact_file") }}</dt>
          <dd>
            <code class="meta-value mono">templates/{{ activeFile }}</code>
            <span v-if="activeRow?.name" class="muted small">
              {{ t("workspace.plugins.heldtemplates.fact_title", [activeRow.name, activeRow.fields]) }}
            </span>
          </dd>
        </div>
        <div>
          <dt class="meta-key small">{{ t("workspace.plugins.heldtemplates.fact_state") }}</dt>
          <dd class="small">
            <template v-if="isPending">{{ t("workspace.plugins.heldtemplates.state_pending") }}</template>
            <template v-else-if="activeRow?.installed && activeRow.differs">{{ t("workspace.plugins.heldtemplates.state_differs") }}</template>
            <template v-else-if="activeRow?.installed">{{ t("workspace.plugins.heldtemplates.state_installed") }}</template>
            <template v-else>{{ t("workspace.plugins.heldtemplates.state_not_installed") }}</template>
          </dd>
        </div>
      </dl>

      <p v-if="activeRow?.error" class="client-error small">{{ activeRow.error }}</p>

      <p v-if="!yaml.trim()" class="muted small">
        {{ t("workspace.plugins.heldtemplates.yaml_placeholder") }}
      </p>
      <CodeEditor
        v-model="yaml"
        lang="yaml"
        :height="420"
        :title="`${activeFile} • ${t('workspace.plugins.tab.heldtemplates')}`"
      />

      <div class="i18n-actions">
        <button
          type="button"
          class="tool-btn primary"
          :disabled="busy || !dirty || !yaml.trim()"
          @click="save"
        >
          {{ t("workspace.plugins.heldtemplates.save") }}
        </button>
        <button
          v-if="activeRow?.installed"
          type="button"
          class="tool-btn"
          :disabled="busy"
          @click="takeOver(activeFile)"
        >
          {{ t("workspace.plugins.heldtemplates.retake") }}
        </button>
        <button
          v-if="activeRow?.installed && activeRow.differs"
          type="button"
          class="tool-btn danger"
          :disabled="busy || !canReplace"
          @click="replace"
        >
          {{ t("workspace.plugins.heldtemplates.replace") }}
        </button>
        <button
          v-else
          type="button"
          class="tool-btn primary"
          :disabled="busy || !canCreate"
          @click="create"
        >
          {{ t("workspace.plugins.heldtemplates.create") }}
        </button>
        <button type="button" class="tool-btn danger" :disabled="busy" @click="remove">
          {{ t("workspace.plugins.heldtemplates.remove") }}
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
