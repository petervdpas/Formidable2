import { ref } from "vue";

// Live bundle zoom, shared so a Settings change rescales the iframe instantly.
export const bundleZoom = ref<number>(1);

// Log failures rather than swallowing them. No visible banner: errors surface
// in a terminal run / devtools instead.
export function reportError(e: unknown): void {
  console.error("[viewer]", e);
}
