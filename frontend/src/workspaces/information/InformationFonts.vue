<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useFonts } from "../../composables/useFonts";
import { useToast } from "../../composables/useToast";
import ConfirmDialog from "../../components/ConfirmDialog.vue";

const { t } = useI18n();
const toast = useToast();

const {
  fonts,
  fontFaceCss,
  loading,
  hasFonts,
  hasSeeds,
  refresh,
  uploadFile,
  removeFont,
  restoreDefaults,
  isSeed,
} = useFonts();

const uploadInput = ref<HTMLInputElement | null>(null);
const uploading = ref(false);
const deleteTarget = ref<string>("");
const deleteOpen = computed(() => deleteTarget.value !== "");
const deleteIsSeed = computed(() => isSeed(deleteTarget.value));

onMounted(() => {
  void refresh();
});

function openPicker() {
  uploadInput.value?.click();
}

async function onFilePicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  uploading.value = true;
  const r = await uploadFile(file);
  uploading.value = false;
  if (r.ok) toast.success("fonts.toast.uploaded", [file.name]);
  else toast.error("fonts.toast.upload_failed", [r.message]);
}

function askDelete(name: string) {
  deleteTarget.value = name;
}

async function confirmDelete() {
  const name = deleteTarget.value;
  const seed = isSeed(name);
  deleteTarget.value = "";
  const r = await removeFont(name);
  if (r.ok) toast.success(seed ? "fonts.toast.reset" : "fonts.toast.deleted", [name]);
  else toast.error("fonts.toast.delete_failed", [r.message]);
}

function cancelDelete() {
  deleteTarget.value = "";
}

async function onRestore() {
  const r = await restoreDefaults();
  if (r.ok) toast.success("fonts.toast.restored");
  else toast.error("fonts.toast.restore_failed", [r.message]);
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <p class="section-info">{{ t('fonts.info') }}</p>

  <!-- @font-face for the uploaded fonts, so each tile can preview in its own face. -->
  <component v-if="fontFaceCss" :is="'style'" v-text="fontFaceCss" />

  <div class="fonts-container">
    <div class="fonts-header">
      <h4>{{ t('fonts.list.title') }}</h4>
      <div class="fonts-header-actions">
        <button class="tool-btn" type="button" :disabled="loading" @click="refresh">
          <i class="fa-solid fa-rotate" aria-hidden="true"></i>
          {{ t('fonts.action.refresh') }}
        </button>
        <button v-if="!hasSeeds" class="tool-btn" type="button" @click="onRestore">
          <i class="fa-solid fa-arrow-rotate-left" aria-hidden="true"></i>
          {{ t('fonts.action.restore') }}
        </button>
        <button class="tool-btn primary" type="button" :disabled="uploading" @click="openPicker">
          <i class="fa-solid fa-upload" aria-hidden="true"></i>
          {{ uploading ? t('fonts.action.uploading') : t('fonts.action.upload') }}
        </button>
        <input
          ref="uploadInput"
          type="file"
          accept=".woff2,.woff,.ttf,.otf,font/woff2,font/woff,font/ttf,font/otf"
          class="fonts-file-input"
          @change="onFilePicked"
        />
      </div>
    </div>

    <p v-if="loading" class="muted small">{{ t('fonts.list.loading') }}</p>
    <p v-else-if="!hasFonts" class="muted small">{{ t('fonts.list.empty') }}</p>

    <ul v-else class="fonts-grid">
      <li v-for="f in fonts" :key="f.filename" :class="['fonts-tile', { seed: f.isSeed }]">
        <div class="fonts-preview" :style="{ fontFamily: `'${f.family}'` }">{{ t('fonts.preview') }}</div>
        <div class="fonts-meta">
          <span class="fonts-family">{{ f.family }}</span>
          <code class="fonts-name">{{ f.filename }}</code>
          <span class="fonts-size">{{ formatBytes(f.size) }}</span>
          <span v-if="f.isSeed" class="fonts-pill">{{ t('fonts.pill.seed') }}</span>
        </div>
        <button
          class="tool-btn fonts-action"
          type="button"
          :title="t(f.isSeed ? 'fonts.action.reset' : 'fonts.action.delete')"
          @click="askDelete(f.filename)"
        >
          <i :class="f.isSeed ? 'fa-solid fa-arrow-rotate-left' : 'fa-solid fa-trash'" aria-hidden="true"></i>
        </button>
      </li>
    </ul>
  </div>

  <ConfirmDialog
    :open="deleteOpen"
    :title="t(deleteIsSeed ? 'fonts.confirm.reset_title' : 'fonts.confirm.delete_title')"
    :message="t(deleteIsSeed ? 'fonts.confirm.reset_message' : 'fonts.confirm.delete_message', [deleteTarget])"
    :confirm-label="t(deleteIsSeed ? 'fonts.action.reset' : 'fonts.action.delete')"
    :cancel-label="t('common.cancel')"
    :variant="deleteIsSeed ? 'default' : 'danger'"
    @cancel="cancelDelete"
    @confirm="confirmDelete"
  />
</template>
