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
    /** Let the user type freely (spaces, punctuation): `slugFn` (a backend call)
     * reduces the stem to the allowed charset before validating, and a live
     * preview shows the resulting filename. When absent, the name is validated
     * as typed. The rule lives in the backend, not here. */
    slugFn?: (raw: string) => Promise<string>;
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

// The typed value with any trailing extension stripped, leaving the bare stem.
function typedStem(): string {
  const raw = name.value.trim();
  const ext = props.extension;
  return ext && raw.endsWith(ext) ? raw.slice(0, -ext.length) : raw;
}

// The stem as it will be stored: slugged by the backend when slugFn is set,
// otherwise the typed stem verbatim.
async function resolveStem(): Promise<string> {
  const stem = typedStem();
  if (!stem || !props.slugFn) return stem;
  return props.slugFn(stem);
}

function withDateAndExt(stem: string): string {
  const dated =
    props.showAppendDate && appendDate.value
      ? `${stem}-${todayYYYYMMDD()}`
      : stem;
  return props.pattern.test(dated) ? `${dated}${props.extension}` : "";
}

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

// Live "Saved as: …" preview (slugging only), recomputed via the backend as the
// user types or toggles the date. Kept in a ref because the slug is async.
const preview = ref("");
watch(
  [name, appendDate, () => props.open],
  async () => {
    if (!props.slugFn || !name.value.trim()) {
      preview.value = "";
      return;
    }
    const typed = name.value; // guard against a stale async resolve
    const filename = withDateAndExt(await resolveStem());
    if (name.value === typed) preview.value = filename;
  },
  { immediate: true },
);

async function onSubmit(): Promise<void> {
  localError.value = "";
  if (!name.value.trim()) {
    localError.value = t("entry_name.error.required");
    return;
  }
  const filename = withDateAndExt(await resolveStem());
  if (!filename) {
    localError.value =
      props.invalidCharsMessage ?? t("entry_name.error.invalid_chars");
    return;
  }
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
    <p v-if="preview" class="muted small">{{ t('entry_name.preview', [preview]) }}</p>
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
