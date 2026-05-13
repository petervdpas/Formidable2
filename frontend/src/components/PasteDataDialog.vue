<script setup lang="ts">
import { ref, watch, nextTick, useTemplateRef } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import { parsePastedRows } from "../utils/pasteData";

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  title?: string;
  subtitle?: string;
}>();

const emit = defineEmits<{
  (e: "process", rows: string[][]): void;
  (e: "cancel"): void;
}>();

const text = ref("");
const textareaRef = useTemplateRef<HTMLTextAreaElement>("textarea");

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      text.value = "";
      await nextTick();
      textareaRef.value?.focus();
    }
  },
);

function onProcess() {
  const { rows } = parsePastedRows(text.value);
  emit("process", rows);
}
</script>

<template>
  <Modal
    :open="open"
    :title="title ?? t('paste.title')"
    width="520px"
    @close="emit('cancel')"
  >
    <p class="paste-subtitle">{{ subtitle }}</p>
    <textarea
      ref="textarea"
      v-model="text"
      class="paste-textarea"
      :placeholder="t('paste.placeholder')"
      spellcheck="false"
    ></textarea>

    <template #footer>
      <button class="tool-btn primary" type="button" @click="onProcess">
        {{ t('paste.process') }}
      </button>
      <button class="tool-btn" type="button" @click="emit('cancel')">
        {{ t('paste.cancel') }}
      </button>
    </template>
  </Modal>
</template>
