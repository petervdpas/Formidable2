import { ref, watchEffect, onBeforeUnmount } from "vue";
import type { MenuEntry } from "../types/menu";

// Module-scope singleton registry. Whichever workspace is currently
// mounted owns the menu; switching workspaces clears it (auto-handled
// by the onBeforeUnmount in setTopbarMenu).
const menus = ref<MenuEntry[]>([]);

/**
 * Register the topbar menu entries for the current workspace. Pass a
 * GETTER, not a value — the getter is re-evaluated whenever any
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
