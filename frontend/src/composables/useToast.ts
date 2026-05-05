import { ref } from "vue";
import { i18n } from "../i18n";

export type ToastVariant = "info" | "success" | "warn" | "error";

export interface Toast {
  id: string;
  variant: ToastVariant;
  /** Already-translated, ready-to-render string. */
  text: string;
  /** ms before auto-dismiss; 0 = sticky. */
  duration: number;
}

export interface ToastOpts {
  /** Override the auto-dismiss duration in ms. Default: 3000. */
  duration?: number;
  /** When true, ignore `duration` and stay until manually dismissed. */
  sticky?: boolean;
  /** Suppress repeats of the same toast within this window (ms). Default: 800. */
  dedupeMs?: number;
  /** Custom dedupe key; defaults to `variant|text`. */
  dedupeKey?: string;
  /** Skip dedupe entirely. */
  force?: boolean;
}

const toasts = ref<Toast[]>([]);
const lastShown = new Map<string, number>();

let counter = 0;
function nextId(): string { counter += 1; return `toast-${counter}`; }

// Heuristic ported from Formidable's Toast facade: "looks like an i18n
// key if it contains a dot and no whitespace." Cheap, good-enough for
// the call sites we control.
function looksLikeI18nKey(s: string): boolean {
  return s.includes(".") && !/\s/.test(s);
}

function translate(keyOrText: string, args?: unknown[]): string {
  if (!keyOrText) return "";
  if (!looksLikeI18nKey(keyOrText)) return keyOrText;
  if (args && args.length) return i18n.global.t(keyOrText, args as never);
  return i18n.global.t(keyOrText);
}

function show(
  variant: ToastVariant,
  keyOrText: string,
  args?: unknown[],
  opts: ToastOpts = {},
): string | null {
  const text = translate(keyOrText, args);
  if (!text) return null;

  const dedupeMs = opts.dedupeMs ?? 800;
  if (!opts.force && dedupeMs > 0) {
    const key = opts.dedupeKey ?? `${variant}|${text}`;
    const now = Date.now();
    const prev = lastShown.get(key) ?? 0;
    if (now - prev < dedupeMs) return null;
    lastShown.set(key, now);
  }

  const duration = opts.sticky ? 0 : (opts.duration ?? 3000);
  const id = nextId();
  toasts.value = [...toasts.value, { id, variant, text, duration }];

  if (duration > 0) {
    setTimeout(() => dismiss(id), duration);
  }
  return id;
}

function dismiss(id: string): void {
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

function clear(): void {
  toasts.value = [];
}

export function useToast() {
  return {
    toasts,
    show,
    dismiss,
    clear,
    info:    (k: string, a?: unknown[], o?: ToastOpts) => show("info",    k, a, o),
    success: (k: string, a?: unknown[], o?: ToastOpts) => show("success", k, a, o),
    warn:    (k: string, a?: unknown[], o?: ToastOpts) => show("warn",    k, a, o),
    error:   (k: string, a?: unknown[], o?: ToastOpts) => show("error",   k, a, o),
  };
}
