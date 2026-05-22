import { computed, ref, watchEffect, onBeforeUnmount } from "vue";
import type { MenuAction, MenuEntry } from "../types/menu";
import { isTypingTarget, matchesCombo, parseCombo, type ParsedCombo } from "../utils/keyboardCombo";

// Module-scope singleton registry. Whichever workspace is currently
// mounted owns the menu; switching workspaces clears it (auto-handled
// by the onBeforeUnmount in setTopbarMenu).
const menus = ref<MenuEntry[]>([]);

/**
 * Register the topbar menu entries for the current workspace. Pass a
 * GETTER, not a value - the getter is re-evaluated whenever any
 * reactive ref it touches changes, so disabled flags / labels stay in
 * sync without manual notifications.
 *
 * Auto-clears on unmount, so the next workspace starts from a clean
 * slate (or doesn't register anything → topbar is empty on the menu
 * side, just like our config-style workspaces today).
 */
export function setTopbarMenu(getter: () => MenuEntry[]): void {
  watchEffect(() => {
    menus.value = getter();
  });
  onBeforeUnmount(() => {
    menus.value = [];
  });
}

/** Read-side: Topbar.vue uses this to render the registered menus. */
export function useTopbarMenu() {
  return { menus };
}

// ── Keyboard shortcuts ──────────────────────────────────────────
//
// Every MenuAction's `combo` is matched against keydown events at
// window level. The current `menus` ref is the single source of
// truth: when a workspace re-registers menus (disabled flags flip,
// items appear/disappear), the shortcut index recomputes via the
// computed below - no manual rebind. We attach the listener once at
// module init; menus.value is empty when no workspace is mounted, so
// the handler is a no-op then.

interface ShortcutBinding {
  parsed: ParsedCombo;
  action: MenuAction;
}

const shortcuts = computed<ShortcutBinding[]>(() => {
  const out: ShortcutBinding[] = [];
  for (const entry of menus.value) {
    if (entry.type === "group") {
      for (const it of entry.items) {
        if (it.type === "separator") continue;
        const parsed = it.combo ? parseCombo(it.combo) : null;
        if (parsed) out.push({ parsed, action: it });
      }
    } else if (entry.combo) {
      const parsed = parseCombo(entry.combo);
      if (parsed) out.push({ parsed, action: entry });
    }
  }
  return out;
});

function handleKeydown(e: KeyboardEvent) {
  const list = shortcuts.value;
  if (list.length === 0) return;
  const typing = isTypingTarget(e.target);
  for (const { parsed, action } of list) {
    if (!matchesCombo(e, parsed)) continue;
    if (action.disabled) return; // claim the combo but no-op
    if (typing && !action.allowWhenTyping) return;
    e.preventDefault();
    e.stopPropagation();
    void action.onClick();
    return;
  }
}

if (typeof window !== "undefined") {
  window.addEventListener("keydown", handleKeydown, true);
}
