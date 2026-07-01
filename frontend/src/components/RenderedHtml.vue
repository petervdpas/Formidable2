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
  // Stash the source once, then reset each block so re-runs (theme
  // switch, content change) re-render from source rather than from the
  // already-injected SVG.
  for (const n of nodes) {
    if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent ?? "";
    n.removeAttribute("data-processed");
    n.textContent = n.dataset.mmsrc;
  }
  try {
    await mermaid.run({ nodes });
  } catch {
    // On a parse error the block keeps its source text; nothing to do.
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
