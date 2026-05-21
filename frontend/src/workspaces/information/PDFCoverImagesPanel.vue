<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { usePDFCoverImages } from "../../composables/usePDFCoverImages";
import { useToast } from "../../composables/useToast";
import ConfirmDialog from "../../components/ConfirmDialog.vue";

const { t } = useI18n();
const toast = useToast();

const {
  images,
  loading,
  hasImages,
  refresh,
  uploadFile,
  removeImage,
  loadDataURL,
  isSeed,
} = usePDFCoverImages();

const uploadInput = ref<HTMLInputElement | null>(null);
const uploading = ref(false);
const deleteTarget = ref<string>("");
const deleteOpen = computed(() => deleteTarget.value !== "");
const deleteIsSeed = computed(() => isSeed(deleteTarget.value));

// Per-image data URL cache. Filled lazily as the tiles mount; refreshed
// when the underlying list changes. Avoids re-fetching the same bytes
// on every render cycle.
const previewByName = ref<Record<string, string>>({});

onMounted(() => {
  void refresh();
});

watch(images, async (next) => {
  const wanted = new Set(next.map((i) => i.name));
  // Drop stale entries.
  for (const k of Object.keys(previewByName.value)) {
    if (!wanted.has(k)) delete previewByName.value[k];
  }
  // Fetch missing previews in parallel.
  await Promise.all(
    next.map(async (img) => {
      if (previewByName.value[img.name]) return;
      try {
        previewByName.value[img.name] = await loadDataURL(img.name);
      } catch {
        previewByName.value[img.name] = "";
      }
    }),
  );
}, { immediate: true });

function openPicker() {
  uploadInput.value?.click();
}

async function onFilePicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = ""; // reset so re-picking the same file fires change again
  if (!file) return;
  uploading.value = true;
  const r = await uploadFile(file);
  uploading.value = false;
  if (r.ok) {
    toast.success("pdf.cover_images.toast.uploaded", [file.name]);
  } else {
    toast.error("pdf.cover_images.toast.upload_failed", [r.message]);
  }
}

function askDelete(name: string) {
  deleteTarget.value = name;
}

async function confirmDelete() {
  const name = deleteTarget.value;
  const seed = isSeed(name);
  deleteTarget.value = "";
  const r = await removeImage(name);
  if (r.ok) {
    toast.success(
      seed ? "pdf.cover_images.toast.reset" : "pdf.cover_images.toast.deleted",
      [name],
    );
  } else {
    toast.error("pdf.cover_images.toast.delete_failed", [r.message]);
  }
}

function cancelDelete() {
  deleteTarget.value = "";
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <p class="section-info">{{ t('pdf.cover_images.info') }}</p>

  <div class="pdf-cover-images-container">
    <div class="pdf-cover-images-header">
      <h4>{{ t('pdf.cover_images.list.title') }}</h4>
      <div class="pdf-cover-images-header-actions">
        <button class="tool-btn" type="button" :disabled="loading" @click="refresh">
          <i class="fa-solid fa-rotate" aria-hidden="true"></i>
          {{ t('pdf.cover_images.action.refresh') }}
        </button>
        <button class="tool-btn primary" type="button" :disabled="uploading" @click="openPicker">
          <i class="fa-solid fa-upload" aria-hidden="true"></i>
          {{ uploading ? t('pdf.cover_images.action.uploading') : t('pdf.cover_images.action.upload') }}
        </button>
        <input
          ref="uploadInput"
          type="file"
          accept=".png,.jpg,.jpeg,.gif,.svg,.webp,image/png,image/jpeg,image/gif,image/svg+xml,image/webp"
          class="pdf-cover-images-file-input"
          @change="onFilePicked"
        />
      </div>
    </div>

    <p v-if="loading" class="muted small">{{ t('pdf.cover_images.list.loading') }}</p>
    <p v-else-if="!hasImages" class="muted small">{{ t('pdf.cover_images.list.empty') }}</p>

    <ul v-else class="pdf-cover-images-grid">
      <li
        v-for="img in images"
        :key="img.name"
        :class="['pdf-cover-images-tile', { seed: img.isSeed }]"
      >
        <div class="pdf-cover-images-thumb">
          <img
            v-if="previewByName[img.name]"
            :src="previewByName[img.name]"
            :alt="img.name"
          />
          <span v-else class="pdf-cover-images-thumb-placeholder">
            <i class="fa-solid fa-image" aria-hidden="true"></i>
          </span>
        </div>
        <div class="pdf-cover-images-meta">
          <code class="pdf-cover-images-name">{{ img.name }}</code>
          <span class="pdf-cover-images-size">{{ formatBytes(img.size) }}</span>
          <span v-if="img.isSeed" class="pdf-cover-images-pill">
            {{ t('pdf.cover_images.pill.seed') }}
          </span>
        </div>
        <button
          class="tool-btn pdf-cover-images-action"
          type="button"
          :title="t(img.isSeed ? 'pdf.cover_images.action.reset' : 'pdf.cover_images.action.delete')"
          @click="askDelete(img.name)"
        >
          <i
            :class="img.isSeed ? 'fa-solid fa-arrow-rotate-left' : 'fa-solid fa-trash'"
            aria-hidden="true"
          ></i>
        </button>
      </li>
    </ul>
  </div>

  <ConfirmDialog
    :open="deleteOpen"
    :title="t(deleteIsSeed ? 'pdf.cover_images.confirm.reset_title' : 'pdf.cover_images.confirm.delete_title')"
    :message="t(deleteIsSeed ? 'pdf.cover_images.confirm.reset_message' : 'pdf.cover_images.confirm.delete_message', [deleteTarget])"
    :confirm-label="t(deleteIsSeed ? 'pdf.cover_images.action.reset' : 'pdf.cover_images.action.delete')"
    :cancel-label="t('common.cancel')"
    :variant="deleteIsSeed ? 'default' : 'danger'"
    @cancel="cancelDelete"
    @confirm="confirmDelete"
  />
</template>
