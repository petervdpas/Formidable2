import { ref } from "vue";
import { i18n } from "../i18n";

export type StatusVariant = "info" | "success" | "warn" | "error" | "create";

export interface StatusOpts {
  /** Colour modifier on the status text. Default "info". */
  variant?: StatusVariant;
  /** ms after which the status reverts to the empty/ready state.
   *  0 = sticky until the next set()/clear(). Default 0. */
  resetMs?: number;
}

// Module-level singleton state. Mirrors the useToast.ts shape so any
// component can `useStatusBar()` and share the same reactive refs.
const text = ref("");
const variant = ref<StatusVariant>("info");
let resetTimer: number | null = null;

// Same i18n-key heuristic as useToast.ts — a dotted, whitespace-free
// string is treated as an i18n key; otherwise it's already-translated.
function looksLikeI18nKey(s: string): boolean {
  return s.includes(".") && !/\s/.test(s);
}

function translate(keyOrText: string, args?: unknown[]): string {
  if (!keyOrText) return "";
  if (!looksLikeI18nKey(keyOrText)) return keyOrText;
  if (args && args.length) return i18n.global.t(keyOrText, args as never);
  return i18n.global.t(keyOrText);
}

function set(keyOrText: string, args?: unknown[], opts: StatusOpts = {}): void {
  const t = translate(keyOrText, args);
  if (!t) return;
  text.value = t;
  variant.value = opts.variant ?? "info";

  if (resetTimer !== null) {
    clearTimeout(resetTimer);
    resetTimer = null;
  }
  const reset = opts.resetMs ?? 0;
  if (reset > 0) {
    resetTimer = window.setTimeout(() => {
      text.value = "";
      variant.value = "info";
      resetTimer = null;
    }, reset);
  }
}

function clear(): void {
  text.value = "";
  variant.value = "info";
  if (resetTimer !== null) {
    clearTimeout(resetTimer);
    resetTimer = null;
  }
}

export function useStatusBar() {
  return { text, variant, set, clear };
}
