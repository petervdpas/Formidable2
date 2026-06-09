import { nextTick, onBeforeUnmount, onMounted } from "vue";
import { scrollToActiveRow } from "../utils/scrollToActiveRow";

// useListKeyNav wires ArrowUp/ArrowDown to step a sidebar list to the
// previous/next item. It is presentation-free behaviour you inject into any
// list-owning workspace: pass the ordered keys, the current key, and a select
// callback. The selection mechanism (and any unsaved-changes guard it runs
// through) stays the caller's concern.
//
// The handler is document-scoped but inert while a text field is focused or a
// modal is open, so it never steals arrow keys from an editor or dialog. It is
// added on mount and removed on unmount, so it only lives while the owning
// workspace is on screen.
export interface ListKeyNavOptions {
  // Ordered keys currently visible in the list (top to bottom).
  keys: () => readonly string[];
  // The selected key, or "" when nothing is selected.
  current: () => string;
  // Move selection to key. May be async; return value is ignored.
  select: (key: string) => void;
  // Optional gate: navigation is skipped when this returns false.
  enabled?: () => boolean;
  // Wrap past the ends instead of stopping (default false).
  wrap?: boolean;
  // Optional scroll container. When given, the stepped-to row is scrolled
  // into view (no-op when already visible). The row is located by its
  // `data-filename` attribute, so the key must match what the list item
  // stamps there. Click selection has its own one-shot scroll; keyboard
  // stepping needs this because the next row is often just off-screen.
  container?: () => HTMLElement | null;
  // Optional index-based scroll, used instead of `container` when the list
  // is virtualized (the stepped-to row may not be in the DOM, so a
  // `data-filename` query can't find it). Takes precedence over `container`.
  scrollTo?: (key: string) => void;
}

const EDITABLE_TAGS = new Set(["INPUT", "TEXTAREA", "SELECT"]);

function inEditable(el: Element | null): boolean {
  for (let n: Element | null = el; n; n = n.parentElement) {
    if (EDITABLE_TAGS.has(n.tagName)) return true;
    if ((n as HTMLElement).isContentEditable) return true;
  }
  return false;
}

function modalOpen(): boolean {
  return document.querySelector('.modal-backdrop, [role="dialog"]') !== null;
}

export function useListKeyNav(opts: ListKeyNavOptions): void {
  function onKey(e: KeyboardEvent): void {
    if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    if (opts.enabled && !opts.enabled()) return;
    if (inEditable(e.target as Element | null)) return;
    if (modalOpen()) return;

    const keys = opts.keys();
    if (keys.length === 0) return;

    const dir = e.key === "ArrowDown" ? 1 : -1;
    const idx = keys.indexOf(opts.current());
    let next: number;
    if (idx === -1) {
      next = dir === 1 ? 0 : keys.length - 1;
    } else {
      next = idx + dir;
      if (next < 0 || next >= keys.length) {
        if (!opts.wrap) return;
        next = (next + keys.length) % keys.length;
      }
    }

    const key = keys[next];
    if (key && key !== opts.current()) {
      e.preventDefault();
      opts.select(key);
      if (opts.scrollTo) {
        void nextTick(() => opts.scrollTo!(key));
      } else {
        const el = opts.container?.();
        if (el) void nextTick(() => scrollToActiveRow(el, key));
      }
    }
  }

  onMounted(() => document.addEventListener("keydown", onKey));
  onBeforeUnmount(() => document.removeEventListener("keydown", onKey));
}
