import { ref } from "vue";

// Shared, visible diagnostics so a failing bound-service call or event never
// fails silently (the viewer has no devtools by default).
export const lastError = ref<string>("");

// Live bundle zoom, shared so a Settings change rescales the iframe instantly.
export const bundleZoom = ref<number>(1);

export function reportError(e: unknown): void {
  const msg = e instanceof Error ? e.message : String(e);
  lastError.value = msg;
  // Also log so it shows in a terminal run.
  console.error("[viewer]", e);
}

export function clearError(): void {
  lastError.value = "";
}
