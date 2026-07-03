<script setup lang="ts">
// Reusable reveal.js deck renderer. Given prebuilt deck HTML (one <section> per
// slide, from render.BuildDeck) plus the canvas dimensions, it owns the whole
// reveal lifecycle: init, aspect-fit, per-slide mermaid hydration, media fill,
// resize re-layout, and teardown. Kept free of any dialog/route specifics so the
// in-app previewer AND the future wiki slides surface can both drive it.
import { ref, watch, nextTick, onBeforeUnmount, computed } from "vue";
import Reveal from "reveal.js";
import "reveal.js/reveal.css";
import { useTheme } from "../composables/useTheme";
import { hydrateKatex } from "../utils/mathHydrate";
import { hydrateMermaid } from "../utils/mermaidHydrate";

const props = withDefaults(
  defineProps<{ html: string; width?: number; height?: number; accent?: string; progress?: number }>(),
  { width: 1280, height: 720, accent: "", progress: 3 },
);

const { theme } = useTheme();

// Deck chrome vars (progress thickness + accent); accent gated by a class so
// reveal's adaptive arrow colour stays when none is set (mirrors deck.css).
const chromeStyle = computed(() => {
  const s: Record<string, string> = {
    aspectRatio: aspect.value,
    "--deck-progress-h": `${props.progress || 3}px`,
  };
  if (props.accent) s["--deck-accent"] = props.accent;
  return s;
});

// Constrain the stage to the canvas aspect so reveal fills it edge-to-edge (like
// the editor) instead of pillarboxing the slide inside the dialog.
const aspect = computed(() => `${props.width || 1280} / ${props.height || 720}`);

const revealEl = ref<HTMLElement | null>(null);
let deck: InstanceType<typeof Reveal> | null = null;
let ro: ResizeObserver | null = null;
let rafId = 0;

function relayout() {
  cancelAnimationFrame(rafId);
  rafId = requestAnimationFrame(() => {
    try {
      deck?.layout();
    } catch {
      /* not initialised yet */
    }
  });
}

async function hydrateSlide(scope?: HTMLElement | null) {
  // mermaid.render builds each diagram off-DOM, so slides need not be visible to
  // hydrate; passing the whole deck renders every slide (incl. overview) up front.
  const el = scope ?? revealEl.value;
  await hydrateKatex(el);
  await hydrateMermaid(el, theme.value === "light" ? "default" : "dark");
}

function destroyReveal() {
  ro?.disconnect();
  ro = null;
  cancelAnimationFrame(rafId);
  if (deck) {
    try {
      deck.destroy();
    } catch {
      /* already torn down */
    }
    deck = null;
  }
}

async function initReveal() {
  destroyReveal();
  await nextTick(); // reveal reads .slides children on initialize
  if (!revealEl.value) return;
  deck = new Reveal(revealEl.value, {
    embedded: true,
    width: props.width || 1280,
    height: props.height || 720,
    margin: 0,
    center: false,
    controls: true,
    progress: true,
    hash: false,
    keyboardCondition: "focused",
  });
  await deck.initialize();
  await hydrateSlide();
  deck.on("slidechanged", (ev) => {
    void hydrateSlide((ev as { currentSlide?: HTMLElement }).currentSlide).then(relayout);
  });
  ro = new ResizeObserver(relayout);
  ro.observe(revealEl.value);
  relayout();
}

watch(
  () => props.html,
  (html) => {
    if (html) void initReveal();
    else destroyReveal();
  },
  { immediate: true },
);
onBeforeUnmount(destroyReveal);
</script>

<template>
  <div ref="revealEl" class="reveal deck-reveal" :class="{ 'deck-accented': !!accent }" :style="chromeStyle">
    <!-- formidable-prose gives the deck the SAME typographic context as the
         editor, overriding reveal's own base font-size (20pt) that otherwise
         cascades into the slide content (e.g. blowing up mermaid text). -->
    <div class="slides formidable-prose" v-html="html"></div>
  </div>
</template>
