import { ref } from "vue";

// The resolved viewer theme ("system" collapsed to light/dark), shared so the
// shell can push it to the bundle iframes (which the user's theme choice
// controls, per-bundle, independent of the always-light standalone wiki).
export const viewerTheme = ref<"light" | "dark">("dark");

// Log failures rather than swallowing them. No visible banner: errors surface
// in a terminal run / devtools instead.
export function reportError(e: unknown): void {
  console.error("[viewer]", e);
}
