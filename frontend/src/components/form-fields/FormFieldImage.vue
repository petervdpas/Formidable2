<script setup lang="ts">
import { computed, inject, ref, watch, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import ImageLightbox from "../ImageLightbox.vue";
import ConfirmDialog from "../ConfirmDialog.vue";
import ImageLibraryDialog from "../ImageLibraryDialog.vue";
import { Service as StorageSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/storage";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { safeFileStem, extensionOf } from "../../utils/imageName";
import { backendErrMessage } from "../../utils/backendError";

// FormFieldImage - picker + preview + clear.
//
// Storage shape: a bare filename string. The image bytes live in
// <storage>/<template>/images/<filename> (storage.SaveImageFile /
// .LoadImageFile / .DeleteImageFile own the lifecycle).
//
// On pick: reads the chosen File as bytes, calls SaveImageFile (which
// overwrites if the same name already exists), sets value to filename.
// On clear: calls DeleteImageFile, sets value to "".
//
// The original lightbox-on-click is deferred - v1 just shows a 200px
// thumbnail.

const props = defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const emit = defineEmits<{ (e: "update:modelValue", v: unknown): void }>();

const { t } = useI18n();

// Provided by StorageWorkspace - the template's YAML filename.
const templateFilename = inject<ComputedRef<string>>(
  "templateFilename",
  computed(() => ""),
);

const filename = computed<string>({
  get: () => (typeof props.modelValue === "string" ? props.modelValue : ""),
  set: (v) => emit("update:modelValue", v),
});

// ── Preview ──────────────────────────────────────────────────────────
// Loads <storage>/<template>/images/<filename> as a data URL and binds
// it to the <img>. Empty filename → no preview.
const dataUrl = ref<string>("");
const loadError = ref<string>("");

async function loadPreview() {
  loadError.value = "";
  if (!filename.value || !templateFilename.value) {
    dataUrl.value = "";
    return;
  }
  try {
    const url = await StorageSvc.LoadImageFile(
      templateFilename.value,
      filename.value,
    );
    dataUrl.value = url ?? "";
  } catch (err) {
    loadError.value = String(err);
    dataUrl.value = "";
  }
}

watch(
  [filename, templateFilename],
  () => {
    void loadPreview();
  },
  { immediate: true },
);

// ── Pick ─────────────────────────────────────────────────────────────
const fileInput = ref<HTMLInputElement | null>(null);
const busy = ref(false);
const pickError = ref<string>("");

const ALLOWED = new Set(["image/png", "image/jpeg"]);

function openPicker() {
  pickError.value = "";
  fileInput.value?.click();
}

async function onFileChosen(evt: Event) {
  const input = evt.target as HTMLInputElement;
  const file = input.files?.[0];
  // Reset so the same file can be re-picked later (browsers suppress
  // change events when selecting the identical file twice in a row).
  input.value = "";
  if (!file) return;

  if (!ALLOWED.has(file.type)) {
    pickError.value = t("workspace.storage.field.image_unsupported");
    return;
  }
  if (!templateFilename.value) {
    pickError.value = t("workspace.storage.field.image_no_template");
    return;
  }

  busy.value = true;
  try {
    const buf = await file.arrayBuffer();
    // Wails marshals Go []byte as a base64 string on the wire - encode
    // here so the binding's `content: string` parameter is accepted.
    const result = await StorageSvc.SaveImageFile(
      templateFilename.value,
      file.name,
      bytesToBase64(new Uint8Array(buf)),
    );
    if (!result?.success) {
      pickError.value = result?.error || t("workspace.storage.field.image_save_failed");
      return;
    }
    filename.value = file.name;
  } catch (err) {
    pickError.value = String(err);
  } finally {
    busy.value = false;
  }
}

// ── Library (pick an existing image, no upload) ──────────────────────
const libraryOpen = ref(false);
function openLibrary() {
  pickError.value = "";
  if (!templateFilename.value) {
    pickError.value = t("workspace.storage.field.image_no_template");
    return;
  }
  libraryOpen.value = true;
}
function onLibraryPick(name: string) {
  libraryOpen.value = false;
  // Reuse by reference: the file already exists, just point the field at it.
  filename.value = name;
}

// ── Rename (moves the file + rewrites references across the template) ──
const renaming = ref(false);
const renameDraft = ref("");

function startRename() {
  if (!filename.value || props.field.readonly || busy.value) return;
  pickError.value = "";
  renameDraft.value = filename.value.slice(
    0,
    filename.value.length - extensionOf(filename.value).length,
  );
  renaming.value = true;
}

function cancelRename() {
  renaming.value = false;
}

// Find a filename not already taken. `keep` is the name we already own (renaming
// onto our own file is free, not a collision).
async function freeName(desired: string, keep: string): Promise<string> {
  if (desired === keep) return desired;
  const ext = extensionOf(desired);
  const stem = ext ? desired.slice(0, desired.length - ext.length) : desired;
  let candidate = desired;
  let n = 1;
  while (
    candidate !== keep &&
    (await StorageSvc.ImageFileExists(templateFilename.value, candidate))
  ) {
    candidate = `${stem}-${n}${ext}`;
    n += 1;
  }
  return candidate;
}

async function commitRename() {
  const stem = safeFileStem(renameDraft.value);
  const old = filename.value;
  if (!stem || !old || !templateFilename.value) {
    renaming.value = false;
    return;
  }
  const desired = `${stem}${extensionOf(old)}`;
  if (desired === old) {
    renaming.value = false;
    return;
  }
  busy.value = true;
  pickError.value = "";
  try {
    const target = await freeName(desired, old);
    await StorageSvc.RenameImageAcrossForms(templateFilename.value, old, target);
    filename.value = target;
    renaming.value = false;
  } catch (err) {
    pickError.value = backendErrMessage(err) || t("workspace.storage.field.image_rename_failed");
  } finally {
    busy.value = false;
  }
}

// ── Clear ────────────────────────────────────────────────────────────
// Encode a Uint8Array as base64 - Wails' []byte parameters expect
// the wire shape to be a base64 string. btoa() handles latin-1 input
// only, so we feed it through fromCharCode in 32k chunks to dodge
// the call-stack ceiling on large blobs.
function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const CHUNK = 0x8000;
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode.apply(
      null,
      Array.from(bytes.subarray(i, i + CHUNK)),
    );
  }
  return btoa(binary);
}

// ── Lightbox (click preview → fullscreen pan/zoom) ──────────────────
const lightboxOpen = ref(false);
function openLightbox() {
  if (dataUrl.value) lightboxOpen.value = true;
}

const confirmClearOpen = ref(false);
function requestClear() {
  if (!filename.value || props.field.readonly || busy.value) return;
  confirmClearOpen.value = true;
}
function cancelClear() {
  confirmClearOpen.value = false;
}
async function confirmClear() {
  confirmClearOpen.value = false;
  pickError.value = "";
  if (!filename.value) return;
  if (templateFilename.value) {
    // Best-effort delete from disk; even if it fails, we still clear
    // the field value so the form can be saved without the reference.
    try {
      await StorageSvc.DeleteImageFile(templateFilename.value, filename.value);
    } catch (err) {
      pickError.value = String(err);
    }
  }
  filename.value = "";
  dataUrl.value = "";
}
</script>

<template>
  <div class="image-field">
    <!-- Hidden input - surfaced via openPicker() so we can show our own
         button styling instead of the browser's default file widget. -->
    <input
      ref="fileInput"
      type="file"
      accept="image/png, image/jpeg"
      class="image-field-hidden"
      :disabled="field.readonly"
      @change="onFileChosen"
    />

    <div v-if="dataUrl" class="image-field-preview-wrap">
      <img
        :src="dataUrl"
        :alt="filename"
        class="image-field-preview"
        :title="t('image_lightbox.click_to_zoom')"
        @click="openLightbox"
      />
    </div>

    <div class="image-field-actions">
      <template v-if="filename && renaming">
        <input
          v-model="renameDraft"
          type="text"
          class="image-field-rename"
          @keydown.enter.prevent="commitRename"
          @keydown.esc.prevent="cancelRename"
        />
        <button type="button" class="tool-btn" :disabled="busy" @click="commitRename">
          {{ t("common.save") }}
        </button>
        <button type="button" class="tool-btn" :disabled="busy" @click="cancelRename">
          {{ t("common.cancel") }}
        </button>
      </template>
      <template v-else>
        <span v-if="filename" class="image-field-filename">{{ filename }}</span>
        <button
          v-if="filename && !field.readonly"
          type="button"
          class="btn-ghost-icon"
          :disabled="busy"
          :title="t('workspace.storage.field.image_rename')"
          @click="startRename"
        >
          <i class="fa-solid fa-pen" aria-hidden="true"></i>
        </button>
      </template>
      <button
        v-if="!field.readonly"
        type="button"
        class="btn-ghost-icon"
        :disabled="busy"
        :title="filename
          ? t('workspace.storage.field.image_replace')
          : t('workspace.storage.field.image_choose')"
        @click="openPicker"
      >
        <i class="fa-solid fa-upload" aria-hidden="true"></i>
      </button>
      <button
        v-if="!field.readonly"
        type="button"
        class="btn-ghost-icon"
        :disabled="busy"
        :title="t('workspace.storage.field.image_library')"
        @click="openLibrary"
      >
        <i class="fa-solid fa-images" aria-hidden="true"></i>
      </button>
      <button
        v-if="filename && !field.readonly"
        type="button"
        class="tool-btn danger"
        :disabled="busy"
        @click="requestClear"
      >
        {{ t("workspace.storage.field.image_clear") }}
      </button>
    </div>

    <p v-if="pickError" class="form-error small">{{ pickError }}</p>
    <p v-else-if="loadError" class="form-error small">{{ loadError }}</p>

    <ImageLightbox
      :open="lightboxOpen"
      :src="dataUrl"
      :alt="filename"
      @close="lightboxOpen = false"
    />

    <ImageLibraryDialog
      :open="libraryOpen"
      :template-filename="templateFilename"
      :current="filename"
      @pick="onLibraryPick"
      @close="libraryOpen = false"
    />

    <ConfirmDialog
      :open="confirmClearOpen"
      :title="t('workspace.storage.field.image_clear.title')"
      :message="t('workspace.storage.field.image_clear.confirm', [filename])"
      :confirm-label="t('workspace.storage.field.image_clear')"
      :cancel-label="t('common.cancel')"
      variant="danger"
      @cancel="cancelClear"
      @confirm="confirmClear"
    />
  </div>
</template>
