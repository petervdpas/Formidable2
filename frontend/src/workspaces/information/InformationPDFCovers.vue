<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { usePDFCovers } from "../../composables/usePDFCovers";
import { useToast } from "../../composables/useToast";
import CodeEditor from "../../components/CodeEditor.vue";
import ConfirmDialog from "../../components/ConfirmDialog.vue";

const { t } = useI18n();
const toast = useToast();
const {
  covers,
  selectedName,
  draftName,
  draftHTML,
  isNew,
  canSave,
  validation,
  saving,
  loading,
  refresh,
  loadCoverForEdit,
  startNew,
  debouncedValidate,
  save,
  remove,
  isSeed,
} = usePDFCovers();

const deleteTarget = ref<string>("");
const deleteOpen = computed(() => deleteTarget.value !== "");
const deleteIsSeed = computed(() => isSeed(deleteTarget.value));

onMounted(() => {
  void refresh();
});

async function onPick(name: string) {
  await loadCoverForEdit(name);
}

async function onSave() {
  const r = await save();
  if (r.ok) {
    toast.success("pdf.covers.toast.saved");
  } else {
    toast.error("pdf.covers.toast.save_failed", [r.message]);
  }
}

function askDelete(name: string) {
  deleteTarget.value = name;
}

async function confirmDelete() {
  const name = deleteTarget.value;
  const seed = isSeed(name);
  deleteTarget.value = "";
  const r = await remove(name);
  if (r.ok) {
    toast.success(seed ? "pdf.covers.toast.reset" : "pdf.covers.toast.deleted");
  } else {
    toast.error("pdf.covers.toast.delete_failed", [r.message]);
  }
}

function cancelDelete() {
  deleteTarget.value = "";
}

function onEditorUpdate(next: string) {
  debouncedValidate(next);
}
</script>

<template>
  <p class="section-info">{{ t('pdf.covers.info') }}</p>

  <div class="pdf-covers-layout">
    <aside class="pdf-covers-list">
      <div class="pdf-covers-list-header">
        <h4>{{ t('pdf.covers.list.title') }}</h4>
        <button class="tool-btn" type="button" @click="startNew">
          <i class="fa-solid fa-plus" aria-hidden="true"></i>
          {{ t('pdf.covers.action.new') }}
        </button>
      </div>

      <p v-if="loading" class="muted small">{{ t('pdf.covers.list.loading') }}</p>
      <p v-else-if="covers.length === 0" class="muted small">
        {{ t('pdf.covers.list.empty') }}
      </p>

      <ul v-else class="pdf-covers-rows">
        <li
          v-for="c in covers"
          :key="c.name"
          :class="['pdf-covers-row', { active: selectedName === c.name, invalid: !c.ok }]"
        >
          <button class="pdf-covers-row-main" type="button" @click="onPick(c.name)">
            <span class="pdf-covers-row-label">
              {{ c.label || c.name }}
              <span v-if="isSeed(c.name)" class="pdf-covers-pill pdf-covers-pill-seed">
                {{ t('pdf.covers.pill.seed') }}
              </span>
              <span v-if="!c.ok" class="pdf-covers-pill pdf-covers-pill-invalid">
                {{ t('pdf.covers.pill.invalid') }}
              </span>
            </span>
            <span v-if="c.description" class="pdf-covers-row-desc">{{ c.description }}</span>
            <code class="pdf-covers-row-name">{{ c.name }}.html</code>
          </button>
          <button
            class="tool-btn pdf-covers-row-delete"
            type="button"
            :title="t(isSeed(c.name) ? 'pdf.covers.action.reset' : 'pdf.covers.action.delete')"
            @click="askDelete(c.name)"
          >
            <i :class="isSeed(c.name) ? 'fa-solid fa-arrow-rotate-left' : 'fa-solid fa-trash'" aria-hidden="true"></i>
          </button>
        </li>
      </ul>
    </aside>

    <section class="pdf-covers-editor">
      <p v-if="!selectedName && !isNew" class="muted">
        {{ t('pdf.covers.editor.idle') }}
      </p>

      <template v-else>
        <div class="pdf-covers-editor-row">
          <label class="pdf-covers-label">{{ t('pdf.covers.editor.name') }}</label>
          <input
            v-model="draftName"
            type="text"
            class="pdf-covers-input"
            :readonly="!isNew"
            :placeholder="t('pdf.covers.editor.name_placeholder')"
          />
          <span v-if="!isNew" class="pdf-covers-hint">
            {{ t('pdf.covers.editor.name_locked') }}
          </span>
        </div>

        <div class="pdf-covers-editor-body">
          <CodeEditor
            :model-value="draftHTML"
            lang="html"
            :height="380"
            @update:model-value="onEditorUpdate"
          />
        </div>

        <div class="pdf-covers-validation">
          <p v-if="!validation" class="muted small">{{ t('pdf.covers.validation.idle') }}</p>
          <p v-else-if="validation.ok && (validation.issues?.length ?? 0) === 0" class="pdf-covers-validation-ok">
            <i class="fa-solid fa-circle-check" aria-hidden="true"></i>
            {{ t('pdf.covers.validation.ok') }}
          </p>
          <ul v-else class="pdf-covers-issues">
            <li
              v-for="(issue, idx) in validation.issues ?? []"
              :key="idx"
              :class="['pdf-covers-issue', issue.severity]"
            >
              <i
                :class="issue.severity === 'error' ? 'fa-solid fa-circle-xmark' : 'fa-solid fa-triangle-exclamation'"
                aria-hidden="true"
              ></i>
              <code class="pdf-covers-issue-code">{{ issue.code }}</code>
              <span class="pdf-covers-issue-msg">{{ issue.message }}</span>
            </li>
          </ul>
        </div>

        <div class="pdf-covers-actions">
          <button
            class="tool-btn primary"
            type="button"
            :disabled="!canSave"
            @click="onSave"
          >
            {{ saving ? t('pdf.covers.action.saving') : t('pdf.covers.action.save') }}
          </button>
        </div>
      </template>
    </section>
  </div>

  <ConfirmDialog
    :open="deleteOpen"
    :title="t(deleteIsSeed ? 'pdf.covers.confirm.reset_title' : 'pdf.covers.confirm.delete_title')"
    :message="t(deleteIsSeed ? 'pdf.covers.confirm.reset_message' : 'pdf.covers.confirm.delete_message', [deleteTarget])"
    :confirm-label="t(deleteIsSeed ? 'pdf.covers.action.reset' : 'pdf.covers.action.delete')"
    :cancel-label="t('common.cancel')"
    :variant="deleteIsSeed ? 'default' : 'danger'"
    @cancel="cancelDelete"
    @confirm="confirmDelete"
  />
</template>
