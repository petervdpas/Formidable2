<script setup lang="ts">
// In-app plan-board previewer: fetch the board for a saved record and hand it to
// the presentational ProjectBoard.
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import ProjectBoard from "./ProjectBoard.vue";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import type { Board } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  template: string;
  datafile: string;
}>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const board = ref<Board | null>(null);
const error = ref("");

async function build() {
  error.value = "";
  board.value = null;
  if (!props.template || !props.datafile) return;
  try {
    board.value = await RenderSvc.BuildBoard(props.template, props.datafile);
  } catch (e) {
    error.value = backendErrMessage(e);
  }
}

watch(() => props.open, (open) => { if (open) void build(); });
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.storage.board.preview.title')"
    width="1000px"
    maximizable="full"
    :close-on-esc="true"
    @close="emit('close')"
  >
    <p v-if="error" class="form-error small">{{ error }}</p>
    <ProjectBoard v-else :board="board" />
  </Modal>
</template>
