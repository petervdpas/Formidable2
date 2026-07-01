<script setup lang="ts">
// In-app reveal.js deck previewer. Given a template + ordered datafiles (a deck,
// from form.DeckOrder / the visible sequence order), it asks the backend to
// render one <section> per slide (RenderSvc.BuildDeck) and drives an embedded
// reveal.js instance inside a maximizable dialog.
import { ref, watch, nextTick, onBeforeUnmount } from "vue";
import { useI18n } from "vue-i18n";
import Reveal from "reveal.js";
import "reveal.js/reveal.css";
import Modal from "./Modal.vue";
import { Service as RenderSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/render";
import { backendErrMessage } from "../utils/backendError";

const props = defineProps<{
  open: boolean;
  template: string;
  datafiles: string[];
}>();
const emit = defineEmits<{ (e: "close"): void }>();

const { t } = useI18n();

const revealEl = ref<HTMLElement | null>(null);
const deckHtml = ref("");
const error = ref("");
let deck: InstanceType<typeof Reveal> | null = null;
let ro: ResizeObserver | null = null;
let rafId = 0;

// Embedded reveal locks its scale from the container size at init; the dialog is
// still settling/animating then (and can be maximized later), so re-lay-out on
// every container resize to keep the slide filling the stage.
function relayout() {
  cancelAnimationFrame(rafId);
  rafId = requestAnimationFrame(() => {
    try {
      deck?.layout();
    } catch {
      /* not initialized yet; ignore */
    }
  });
}

function destroyReveal() {
  ro?.disconnect();
  ro = null;
  cancelAnimationFrame(rafId);
  if (deck) {
    try {
      deck.destroy();
    } catch {
      /* reveal throws if already torn down; ignore */
    }
    deck = null;
  }
}

async function buildAndInit() {
  error.value = "";
  deckHtml.value = "";
  destroyReveal();
  if (!props.template || props.datafiles.length === 0) {
    error.value = t("workspace.storage.deck.preview.empty");
    return;
  }
  try {
    const built = await RenderSvc.BuildDeck(props.template, props.datafiles);
    deckHtml.value = built.html || "";
    // Reveal reads .slides children on initialize, so the HTML must be in the
    // DOM first.
    await nextTick();
    if (!revealEl.value) return;
    deck = new Reveal(revealEl.value, {
      embedded: true,
      width: built.width || 1280,
      height: built.height || 720,
      margin: 0,
      center: false,
      controls: true,
      progress: true,
      hash: false,
      keyboardCondition: "focused",
    });
    await deck.initialize();
    // Re-fit whenever the dialog resizes (open animation, maximize, window).
    ro = new ResizeObserver(relayout);
    ro.observe(revealEl.value);
    relayout();
  } catch (e) {
    error.value = backendErrMessage(e);
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) void buildAndInit();
    else destroyReveal();
  },
);
onBeforeUnmount(destroyReveal);
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
      <div v-show="!error" ref="revealEl" class="reveal deck-reveal">
        <div class="slides" v-html="deckHtml"></div>
      </div>
    </div>
  </Modal>
</template>
