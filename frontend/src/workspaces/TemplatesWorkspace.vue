<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import Modal from "../components/Modal.vue";
import { FormSection, FormRow } from "../components/fields";
import { useTemplates, isValidTemplateFilename } from "../composables/useTemplates";
import { useRestartGate } from "../composables/useRestartGate";
import { useToast } from "../composables/useToast";
import { setTopbarMenu } from "../composables/useTopbarMenu";

const { t } = useI18n();
const { bootConfig } = useRestartGate();
const toast = useToast();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

const {
  filenames,
  cache,
  selectedFilename,
  selectedTemplate,
  refresh,
  create,
} = useTemplates();

function displayName(filename: string): string {
  const cached = cache.value.get(filename);
  if (cached?.name && cached.name.trim()) return cached.name;
  return filename.replace(/\.yaml$/, "");
}

// ── Refresh feedback ──────────────────────────────────────────────────
async function doRefresh() {
  try {
    await refresh();
    toast.success("toast.refresh.success");
  } catch (err) {
    toast.error("toast.refresh.error", [String(err)]);
  }
}

// ── Create modal ─────────────────────────────────────────────────────
const createOpen = ref(false);
const createInput = ref("");
const createError = ref<string>("");

function openCreate() {
  createInput.value = "";
  createError.value = "";
  createOpen.value = true;
}

async function submitCreate() {
  const name = createInput.value.trim();
  if (!isValidTemplateFilename(name)) {
    createError.value = t("workspace.templates.create.invalid");
    return;
  }
  const result = await create(name);
  if (!result.ok) {
    createError.value = result.code === "exists"
      ? t("workspace.templates.create.exists")
      : t("workspace.templates.create.error", [result.message ?? "?"]);
    return;
  }
  toast.success("workspace.templates.create.success", [name.replace(/\.yaml$/, "")]);
  createOpen.value = false;
}

// ── Topbar menu ──────────────────────────────────────────────────────
setTopbarMenu(() => [
  {
    type: "group",
    id: "file",
    labelKey: "menu.file",
    items: [
      {
        id: "refresh",
        labelKey: "common.refresh",
        onClick: doRefresh,
      },
    ],
  },
]);
</script>

<template>
  <Teleport defer to="#topbar-content">
    <span class="topbar-spacer"></span>
    <div class="topbar-actions">
      <button class="tool-btn primary" @click="openCreate">
        + {{ t('workspace.templates.new_template') }}
      </button>
    </div>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.templates.sidebar_title') }}</h2>

      <p v-if="filenames.length === 0" class="muted small">
        {{ t('workspace.templates.empty') }}
      </p>

      <ul v-else class="template-list">
        <li
          v-for="f in filenames"
          :key="f"
          :class="['template-list-item', { active: f === selectedFilename }]"
          @click="selectedFilename = f"
        >
          <span class="template-display">{{ displayName(f) }}</span>
          <span class="template-meta">
            <span class="badge small template-filename">{{ f }}</span>
          </span>
        </li>
      </ul>
    </template>

    <template #main>
      <p v-if="!selectedTemplate" class="muted">
        {{ t('workspace.templates.unselected') }}
      </p>

      <template v-else>
        <h1 class="workspace-heading">{{ selectedTemplate.name || selectedFilename }}</h1>
        <div class="template-detail-meta">
          <span class="badge badge-accent">{{ selectedFilename }}</span>
        </div>

        <FormSection :title="t('workspace.templates.setup.title')">
          <FormRow :label="t('workspace.templates.setup.template_name')">
            <span class="readout">{{ selectedTemplate.name || '—' }}</span>
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.item_field')">
            <span class="readout">{{ selectedTemplate.item_field || '—' }}</span>
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.sidebar_expression')">
            <span class="readout">
              <code v-if="selectedTemplate.sidebar_expression">{{ selectedTemplate.sidebar_expression }}</code>
              <template v-else>—</template>
            </span>
          </FormRow>
          <FormRow :label="t('workspace.templates.setup.enable_collection')">
            <span class="readout">{{ selectedTemplate.enable_collection ? t('common.enabled') : t('common.disabled') }}</span>
          </FormRow>
        </FormSection>

        <FormSection :title="t('workspace.templates.fields.title')">
          <p v-if="!selectedTemplate.fields || selectedTemplate.fields.length === 0" class="muted small">
            {{ t('workspace.templates.fields.empty') }}
          </p>
          <ul v-else class="field-rows">
            <li
              v-for="(f, i) in selectedTemplate.fields"
              :key="(f.key || '') + ':' + i"
              class="field-row"
            >
              <span class="field-row-key">{{ f.key || `(field ${i + 1})` }}</span>
              <span class="badge field-row-type">{{ (f.type || '').toUpperCase() }}</span>
              <span v-if="f.primary_key" class="badge badge-ok small">PRIMARY</span>
            </li>
          </ul>
        </FormSection>
      </template>
    </template>
  </SplitPane>

  <!-- Create modal -->
  <Modal
    :open="createOpen"
    :title="t('workspace.templates.create.title')"
    @close="createOpen = false"
  >
    <label class="create-row">
      <span class="create-label">{{ t('workspace.templates.create.label') }}</span>
      <input
        class="field-input"
        v-model="createInput"
        :placeholder="t('workspace.templates.create.placeholder')"
        @keydown.enter="submitCreate"
      />
    </label>
    <p class="muted small create-help">
      {{ t('workspace.templates.create.help') }}
    </p>
    <p v-if="createError" class="form-error">{{ createError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="createOpen = false">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="submitCreate">
        {{ t('workspace.templates.new_template') }}
      </button>
    </template>
  </Modal>
</template>

<style scoped>
/* Sidebar list — same visual language as the profile list. */
.template-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.template-list-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-2);
    border-radius: var(--radius-md);
    cursor: pointer;
    color: var(--color-text);
    background: transparent;
}

.template-list-item:hover { background: var(--list-hover-bg); }

.template-list-item.active {
    background: var(--list-active-bg);
    color: var(--list-active-fg);
}

.template-display { font-weight: 600; }

.template-meta {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-wrap: wrap;
}

.template-filename {
    font-family: var(--font-mono);
    font-size: 11px;
}

/* Main panel */
.template-detail-meta {
    display: flex;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
    flex-wrap: wrap;
}

.readout {
    color: var(--color-text);
    padding-top: 7px;            /* line up with adjacent inputs in mixed forms */
    line-height: 1.4;
}

.readout code {
    font-family: var(--font-mono);
    font-size: 13px;
    background: var(--color-surface);
    padding: 1px 6px;
    border-radius: var(--radius-sm);
}

/* Field rows — Formidable's "key (TYPE) … primary" look. */
.field-rows {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.field-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 8px var(--space-3);
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
}

.field-row-key {
    font-weight: 600;
    flex: 1 1 auto;
    font-family: var(--font-mono);
    font-size: 13px;
}

.field-row-type {
    font-family: var(--font-mono);
    font-size: 11px;
    letter-spacing: 0.04em;
}

/* Create modal */
.create-row {
    display: flex;
    flex-direction: column;
    gap: 6px;
}

.create-label {
    font-weight: 600;
    font-size: var(--font-size-sm);
}

.create-help {
    margin: var(--space-2) 0 0;
}
</style>
