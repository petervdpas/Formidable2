import { ref } from "vue";
import { i18n } from "../i18n";
import { useConfig } from "./useConfig";
import { useStatusBar, type StatusVariant } from "./useStatusBar";

const TOAST_FALLBACK_MS = 5000;

export type ToastVariant = "info" | "success" | "warn" | "error";

/** An inline call-to-action rendered beside the toast text (e.g. the
 *  "Open Collaboration" button on the behind-remote nudge). `label` is
 *  already-translated; `run` fires on click, after which the toast dismisses. */
export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: string;
  variant: ToastVariant;
  /** Already-translated, ready-to-render string. */
  text: string;
  /** ms before auto-dismiss; 0 = sticky. */
  duration: number;
  /** Optional inline action button. */
  action?: ToastAction;
}

export interface ToastOpts {
  /** Override the auto-dismiss duration in ms. Default reads from
   *  config.toast_timeout (seconds, clamped 2-15 server-side). */
  duration?: number;
  /** When true, ignore `duration` and stay until manually dismissed. */
  sticky?: boolean;
  /** Suppress repeats of the same toast within this window (ms). Default: 800. */
  dedupeMs?: number;
  /** Custom dedupe key; defaults to `variant|text`. */
  dedupeKey?: string;
  /** Skip dedupe entirely. */
  force?: boolean;
  /** Also mirror to the status bar. i18n key or pre-translated string. */
  status?: string;
  /** i18n args for `status`. Ignored when `status` is not set. */
  statusArgs?: unknown[];
  /** Override the status-bar colour. Defaults to the toast variant.
   *  Use this when the toast is e.g. `success` but the statusbar
   *  should read as `create` (blue) instead of `success` (green). */
  statusVariant?: StatusVariant;
  /** ms after which the status bar reverts to "ready". 0 = sticky.
   *  Default 0 (status persists until the next set/clear). */
  statusResetMs?: number;
  /** Optional inline call-to-action button. */
  action?: ToastAction;
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

  const { config } = useConfig();
  const configured = config.value?.toast_timeout;
  const defaultMs = configured && configured > 0 ? configured * 1000 : TOAST_FALLBACK_MS;
  const duration = opts.sticky ? 0 : (opts.duration ?? defaultMs);
  const id = nextId();
  toasts.value = [...toasts.value, { id, variant, text, duration, action: opts.action }];

  if (duration > 0) {
    setTimeout(() => dismiss(id), duration);
  }

  // Optional status-bar pass-through. Defaults the status variant to
  // the toast variant; opts.statusVariant overrides (e.g. a `success`
  // toast paired with a `create` statusbar tint for new-record events).
  // resetMs is left undefined so useStatusBar's default revert window
  // wins - callers don't have to know the timing constant.
  if (opts.status) {
    const statusBar = useStatusBar();
    statusBar.set(opts.status, opts.statusArgs, {
      variant: opts.statusVariant ?? (variant as StatusVariant),
      resetMs: opts.statusResetMs,
    });
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
