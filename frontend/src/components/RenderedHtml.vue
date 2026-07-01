<script setup lang="ts">
import { ref, watch, nextTick, onBeforeUnmount } from "vue";
import { useTheme } from "../composables/useTheme";
import { hydrateKatex } from "../utils/mathHydrate";
import { hydrateMermaid } from "../utils/mermaidHydrate";

// Renders backend-produced HTML and hydrates any `<pre class="mermaid">`
// blocks (emitted by the goldmark client-mode extension) into diagrams via the
// shared, isolated mermaid routine. Reusable by every surface that shows
// RenderHTML output.
const props = defineProps<{ html: string }>();

const root = ref<HTMLElement | null>(null);
const { theme } = useTheme();

let seq = 0;
async function hydrate() {
  const mine = ++seq;
  await hydrateMermaid(
    root.value,
    theme.value === "light" ? "default" : "dark",
    () => mine === seq && !!root.value,
  );
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
