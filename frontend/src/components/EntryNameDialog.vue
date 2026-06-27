<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { SwitchField } from "./fields";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    confirmLabel: string;
    /** Filenames already present, used to refuse duplicates client-side. */
    existingNames?: string[];
    placeholder?: string;
    /** Prefilled stem (e.g. the source name for a copy). */
    initialName?: string;
    /** Appended to the stem to form the filename. Empty = the user types the
     * full name (including any extension) and it is validated as-is. */
    extension?: string;
    /** Show the "append date" toggle (off for names that aren't dated). */
    showAppendDate?: boolean;
    /** Pattern the name must match: the stem when extension is set, otherwise
     * the whole typed name. */
    pattern?: RegExp;
    /** Override the invalid-characters message for context-specific rules. */
    invalidCharsMessage?: string;
    /** Help text shown under the input. */
    help?: string;
    /** Parent-controlled error (e.g. a backend failure), shown in the dialog. */
    error?: string;
  }>(),
  {
    existingNames: () => [],
    extension: ".meta.json",
    showAppendDate: true,
    pattern: () => /^[a-zA-Z0-9._-]+$/,
  },
);

const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "submit", filename: string): void;
}>();

const name = ref("");
const appendDate = ref(false);
const localError = ref("");

// Reset on open so a previous attempt can't bleed into a fresh click.
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      name.value = props.initialName ?? "";
      appendDate.value = false;
      localError.value = "";
    }
  },
  { immediate: true },
);

// Local validation wins while present; otherwise show the parent's error.
const shownError = computed(() => localError.value || props.error || "");

// "YYYYMMDD" suffix from today's date (local time - matches the original
// Formidable, which also uses the local-zone date for filenames).
function todayYYYYMMDD(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}${m}${day}`;
}

function onSubmit(): void {
  localError.value = "";
  const raw = name.value.trim();
  if (!raw) {
    localError.value = t("entry_name.error.required");
    return;
  }
  const ext = props.extension;
  const stem = ext && raw.endsWith(ext) ? raw.slice(0, -ext.length) : raw;
  const dated =
    props.showAppendDate && appendDate.value
      ? `${stem}-${todayYYYYMMDD()}`
      : stem;
  if (!props.pattern.test(dated)) {
    localError.value =
      props.invalidCharsMessage ?? t("entry_name.error.invalid_chars");
    return;
  }
  const filename = `${dated}${ext}`;
  if (props.existingNames.includes(filename)) {
    localError.value = t("entry_name.error.exists");
    return;
  }
  emit("submit", filename);
}
</script>

<template>
  <Modal :open="open" :title="title" @close="emit('cancel')">
    <div class="dialog-grid">
      <label class="dialog-grid-label" for="entry-name">
        {{ t('entry_name.label') }}
      </label>
      <input
        id="entry-name"
        class="field-input"
        v-model="name"
        :placeholder="placeholder"
        @keydown.enter="onSubmit"
      />

      <template v-if="showAppendDate">
        <span class="dialog-grid-label">
          {{ t('entry_name.append_date') }}
        </span>
        <SwitchField v-model="appendDate" />
      </template>
    </div>
    <p v-if="help" class="muted small">{{ help }}</p>
    <p v-if="shownError" class="form-error">{{ shownError }}</p>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button class="tool-btn primary" type="button" @click="onSubmit">
        {{ confirmLabel }}
      </button>
    </template>
  </Modal>
</template>
