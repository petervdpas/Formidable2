<script setup lang="ts">
import { computed } from "vue";

// Icon — renders a Flaticon SVG inline so its built-in colors flow.
// Drop new SVGs into src/assets/icons/ and reference by basename:
//   <Icon name="database" />
//
// Vite eager-globs all icons at build time, so unused ones still ship.
// That's fine for a fixed-size set; if it grows large we can switch to
// a lazy import.

const props = withDefaults(
  defineProps<{
    name: string;
    /** Pixel size — drives wrapper width/height. SVGs scale to fit. */
    size?: number;
    /** Optional title for tooltip / a11y. */
    title?: string;
  }>(),
  { size: 24 },
);

const svgs = import.meta.glob("../assets/icons/*.svg", {
  eager: true,
  query: "?raw",
  import: "default",
}) as Record<string, string>;

const svgMap = (() => {
  const out: Record<string, string> = {};
  for (const [path, raw] of Object.entries(svgs)) {
    const m = path.match(/\/([^/]+)\.svg$/);
    if (m) out[m[1]] = raw;
  }
  return out;
})();

const svg = computed(() => svgMap[props.name] ?? "");

const style = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
}));
</script>

<template>
  <span class="icon" :style="style" :title="title" v-html="svg" />
</template>
