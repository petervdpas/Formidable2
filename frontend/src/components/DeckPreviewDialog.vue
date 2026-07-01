<script setup lang="ts">
// In-app deck previewer: fetch the deck HTML for a template + ordered datafiles
// (render.BuildDeck) and hand it to the reusable RevealDeck. All reveal
// lifecycle lives in RevealDeck, so this is just fetch + modal shell.
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Modal from "./Modal.vue";
import RevealDeck from "./RevealDeck.vue";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  template: string;
  datafiles: string[];
}>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const built = ref<{ html: string; width: number; height: number } | null>(null);
const error = ref("");

async function build() {
  error.value = "";
  built.value = null;
  if (!props.template || props.datafiles.length === 0) {
    error.value = t("workspace.storage.deck.preview.empty");
    return;
  }
  try {
    const d = await RenderSvc.BuildDeck(props.template, props.datafiles);
    built.value = { html: d.html || "", width: d.width || 1280, height: d.height || 720 };
  } catch (e) {
    error.value = backendErrMessage(e);
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) void build();
    else built.value = null;
  },
);
</script>

<template>
  <Modal
    :open="open"
    :title="t('workspace.storage.deck.preview.title')"
    maximizable="full"
    :close-on-esc="true"
    @close="emit('close')"
  >
    <div class="deck-preview">
      <p v-if="error" class="form-error small">{{ error }}</p>
      <RevealDeck
        v-else-if="built"
        :html="built.html"
        :width="built.width"
        :height="built.height"
      />
    </div>
  </Modal>
</template>
