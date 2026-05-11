import { ref } from "vue";

export type StatusVariant = "info" | "success" | "warn" | "error" | "create";

// Default auto-revert window. Owned by the status component itself
// rather than the call sites — callers shouldn't have to know how
// long a status lingers. Pass `resetMs: 0` to opt into sticky mode.
export const STATUS_DEFAULT_RESET_MS = 15000;

export interface StatusOpts {
  /** Colour modifier on the status text. Default "info". */
  variant?: StatusVariant;
  /** ms after which the status reverts to the empty/ready state.
   *  0 = sticky until the next set()/clear(). Default
   *  STATUS_DEFAULT_RESET_MS (15s). */
  resetMs?: number;
}

// Module-level singleton state. Mirrors the useToast.ts shape so any
// component can `useStatusBar()` and share the same reactive refs.
// Storing the i18n key + args (rather than the pre-translated string)
// lets Footer.vue render with <i18n-t> and wrap each arg in <strong>
// — that's why filenames render bold without us touching the locale
// files or piping HTML through the composable.
const i18nKey = ref<string | null>(null);
const i18nArgs = ref<unknown[]>([]);
const literal = ref("");
const variant = ref<StatusVariant>("info");
let resetTimer: number | null = null;

// Same i18n-key heuristic as useToast.ts — a dotted, whitespace-free
// string is treated as an i18n key; otherwise it's already-translated.
function looksLikeI18nKey(s: string): boolean {
  return s.includes(".") && !/\s/.test(s);
}

function set(keyOrText: string, args?: unknown[], opts: StatusOpts = {}): void {
  if (!keyOrText) return;

  if (looksLikeI18nKey(keyOrText)) {
    i18nKey.value = keyOrText;
    i18nArgs.value = args ?? [];
    literal.value = "";
  } else {
    i18nKey.value = null;
    i18nArgs.value = [];
    literal.value = keyOrText;
  }
  variant.value = opts.variant ?? "info";

  if (resetTimer !== null) {
    clearTimeout(resetTimer);
    resetTimer = null;
  }
  const reset = opts.resetMs ?? STATUS_DEFAULT_RESET_MS;
  if (reset > 0) {
    resetTimer = window.setTimeout(() => {
      clear();
    }, reset);
  }
}

function clear(): void {
  i18nKey.value = null;
  i18nArgs.value = [];
  literal.value = "";
  variant.value = "info";
  if (resetTimer !== null) {
    clearTimeout(resetTimer);
    resetTimer = null;
  }
}

// Canonical "<verb>: <filename>" subroutines used by both
// TemplatesWorkspace and StorageWorkspace. Filename should be the
// on-disk filename (e.g. "fcdm-enums.yaml" /
// "projectstatus.meta.json"); the Footer wraps it in <strong>.
// Variants pick the theme-driven colour: success→green, error→red,
// create→accent/blue. The 15s revert window is owned by `set()` via
// STATUS_DEFAULT_RESET_MS — call sites just say what happened.
function setSelected(filename: string): void {
  if (!filename) return;
  set("status.selected", [filename]);
}
function setSaved(filename: string): void {
  if (!filename) return;
  set("status.saved", [filename], { variant: "success" });
}
function setDeleted(filename: string): void {
  if (!filename) return;
  set("status.deleted", [filename], { variant: "error" });
}
function setCreated(filename: string): void {
  if (!filename) return;
  set("status.created", [filename], { variant: "create" });
}

export function useStatusBar() {
  return {
    i18nKey,
    i18nArgs,
    literal,
    variant,
    set,
    setSelected,
    setSaved,
    setDeleted,
    setCreated,
    clear,
  };
}
