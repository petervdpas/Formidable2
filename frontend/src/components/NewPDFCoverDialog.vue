<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { coverSlug } from "../composables/usePDFCovers";

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  /** Existing cover filenames (without `.html`) - used to refuse duplicates. */
  existingNames: string[];
}>();

const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "create", name: string): void;
}>();

const name = ref("");

// Reset the field whenever the dialog opens so the previous attempt
// doesn't bleed into a fresh New click.
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) name.value = "";
  },
);

const slug = computed(() => coverSlug(name.value));

const slugInvalid = computed(() => !!name.value.trim() && slug.value === "");
const nameTaken = computed(
  () => !!slug.value && props.existingNames.includes(slug.value),
);
const canCreate = computed(() => !!slug.value && !nameTaken.value && !slugInvalid.value);

function onCreate() {
  if (!canCreate.value) return;
  emit("create", slug.value);
}
</script>

<template>
  <Modal
    :open="open"
    :title="t('pdf.covers.new.dialog.title')"
    width="480px"
    @close="emit('cancel')"
  >
    <p class="muted small">{{ t('pdf.covers.new.dialog.intro') }}</p>

    <div class="new-cover-dialog-row">
      <label class="new-cover-dialog-label" for="new-cover-name">
        {{ t('pdf.covers.new.dialog.name_label') }}
      </label>
      <input
        id="new-cover-name"
        v-model="name"
        type="text"
        class="new-cover-dialog-input"
        :placeholder="t('pdf.covers.editor.name_placeholder')"
        autofocus
        @keydown.enter.prevent="onCreate"
      />
    </div>

    <p v-if="slugInvalid" class="form-error">
      {{ t('pdf.covers.new.dialog.invalid') }}
    </p>
    <p v-else-if="nameTaken" class="form-error">
      {{ t('pdf.covers.new.dialog.name_taken', [slug]) }}
    </p>
    <p v-else-if="slug && slug !== name.trim()" class="muted small">
      <code>{{ slug }}.html</code>
    </p>

    <template #footer>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('common.cancel') }}
      </button>
      <button
        class="tool-btn primary"
        type="button"
        :disabled="!canCreate"
        @click="onCreate"
      >
        {{ t('pdf.covers.new.dialog.create') }}
      </button>
    </template>
  </Modal>
</template>
