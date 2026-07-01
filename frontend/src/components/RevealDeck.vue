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

const props = withDefaults(
  defineProps<{ html: string; width?: number; height?: number }>(),
  { width: 1280, height: 720 },
);

const { theme } = useTheme();

// Constrain the stage to the canvas aspect so reveal fills it edge-to-edge (like
// the editor) instead of pillarboxing the slide inside the dialog.
const aspect = computed(() => `${props.width || 1280} / ${props.height || 720}`);

const revealEl = ref<HTMLElement | null>(null);
let deck: InstanceType<typeof Reveal> | null = null;
let ro: ResizeObserver | null = null;
let rafId = 0;

// Mermaid blocks arrive as static `<pre class="mermaid">`; hydrate lazily.
type MermaidAPI = (typeof import("mermaid"))["default"];
let mermaidPromise: Promise<MermaidAPI> | null = null;
function loadMermaid() {
  if (!mermaidPromise) mermaidPromise = import("mermaid").then((m) => m.default);
  return mermaidPromise;
}

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
  const el = scope ?? (deck?.getCurrentSlide?.() as HTMLElement | undefined) ?? revealEl.value;
  await hydrateKatex(el);
  await hydrateMermaid(el);
}

async function hydrateMermaid(scope?: HTMLElement | null) {
  // Only the visible slide can be measured; a mermaid block on a display:none
  // slide renders broken, so hydrate per-slide as each becomes current.
  const el = scope ?? (deck?.getCurrentSlide?.() as HTMLElement | undefined) ?? revealEl.value;
  if (!el) return;
  const nodes = Array.from(el.querySelectorAll<HTMLElement>(".mermaid"));
  if (nodes.length === 0) return;
  const mermaid = await loadMermaid();
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: theme.value === "light" ? "default" : "dark",
  });
  for (const n of nodes) {
    if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent ?? "";
    n.removeAttribute("data-processed");
    n.textContent = n.dataset.mmsrc;
  }
  try {
    await mermaid.run({ nodes });
  } catch {
    /* parse error: keep source text */
  }
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
    progress: false,
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
  <div ref="revealEl" class="reveal deck-reveal" :style="{ aspectRatio: aspect }">
    <!-- formidable-prose gives the deck the SAME typographic context as the
         editor, overriding reveal's own base font-size (20pt) that otherwise
         cascades into the slide content (e.g. blowing up mermaid text). -->
    <div class="slides formidable-prose" v-html="html"></div>
  </div>
</template>
