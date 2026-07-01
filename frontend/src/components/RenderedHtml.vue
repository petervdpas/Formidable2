<script setup lang="ts">
import { ref, watch, nextTick, onBeforeUnmount } from "vue";
import { useTheme } from "../composables/useTheme";
import { hydrateKatex } from "../utils/mathHydrate";

// Renders backend-produced HTML and hydrates any `<pre class="mermaid">`
// blocks (emitted by the goldmark client-mode extension) into diagrams.
// Reusable by every surface that shows RenderHTML output. mermaid is
// lazy-loaded and shared across instances.
const props = defineProps<{ html: string }>();

const root = ref<HTMLElement | null>(null);
const { theme } = useTheme();

type MermaidAPI = (typeof import("mermaid"))["default"];
let mermaidPromise: Promise<MermaidAPI> | null = null;
function loadMermaid(): Promise<MermaidAPI> {
  if (!mermaidPromise) mermaidPromise = import("mermaid").then((m) => m.default);
  return mermaidPromise;
}

let seq = 0;
async function hydrate() {
  const el = root.value;
  if (!el) return;
  const nodes = Array.from(el.querySelectorAll<HTMLElement>(".mermaid"));
  if (nodes.length === 0) return;
  const mine = ++seq;
  const mermaid = await loadMermaid();
  if (mine !== seq || !root.value) return;
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: theme.value === "light" ? "default" : "dark",
  });
  // Render each block via render(), NOT run(): render() builds the SVG in a
  // clean off-DOM container, so text is measured correctly regardless of any CSS
  // transform/scale on the host (the editor's scaled stage, reveal's transformed
  // slides). run() renders in place and mis-measures there, clipping node text;
  // it also shares mutable global state with render(), so mixing the two APIs
  // across surfaces corrupts each other. One API everywhere = one clean path.
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent ?? "";
    const src = n.dataset.mmsrc;
    try {
      const out = await mermaid.render(`rh-mermaid-${mine}-${i}`, src);
      if (mine !== seq) return; // superseded by a newer hydrate mid-flight
      n.innerHTML = out.svg;
      n.setAttribute("data-processed", "true");
      out.bindFunctions?.(n);
    } catch {
      // Parse error: keep the source text.
      n.textContent = src;
    }
  }
}

watch(
  () => [props.html, theme.value],
  () => {
    void nextTick(() => {
      void hydrate();
      void hydrateKatex(root.value);
    });
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  seq++;
});
</script>

<template>
  <!-- backend HTML; mermaid blocks hydrate with securityLevel "strict". -->
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div ref="root" v-html="html"></div>
</template>
