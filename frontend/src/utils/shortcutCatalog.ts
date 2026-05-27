// Catalog of advertised keyboard shortcuts, surfaced on the
// Information → Keyboard Shortcuts page. Wiring a shortcut on a
// menu action is the source of truth at runtime (see useTopbarMenu);
// this catalog is the *display* surface so users can see every
// combo without having to open the right workspace and click the
// right menu.
//
// Add a row here when you wire a new `combo` field on a MenuAction -
// they aren't auto-discovered (menus only exist while their
// workspace is mounted), so keep the two lists in sync by hand.

export interface ShortcutCatalogEntry {
  /** Cross-platform combo, same syntax as MenuAction.combo. */
  combo: string;
  /** i18n key for the human-readable description. */
  descriptionKey: string;
  /** Optional i18n hint about WHERE the shortcut applies (e.g.
   * "While editing a template"). Empty means "anywhere". */
  scopeKey?: string;
}

export interface ShortcutCatalogGroup {
  /** i18n key for the group heading (usually a workspace name). */
  titleKey: string;
  items: ShortcutCatalogEntry[];
}

export const SHORTCUT_CATALOG: ShortcutCatalogGroup[] = [
  {
    titleKey: "workspace.information.shortcuts.group.templates",
    items: [
      { combo: "Mod+S", descriptionKey: "workspace.information.shortcuts.action.save_template" },
      { combo: "Mod+N", descriptionKey: "workspace.information.shortcuts.action.new_template" },
      { combo: "Mod+D", descriptionKey: "workspace.information.shortcuts.action.delete_template" },
      { combo: "ArrowUp", descriptionKey: "workspace.information.shortcuts.action.prev_template", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
      { combo: "ArrowDown", descriptionKey: "workspace.information.shortcuts.action.next_template", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
    ],
  },
  {
    titleKey: "workspace.information.shortcuts.group.storage",
    items: [
      { combo: "Mod+S", descriptionKey: "workspace.information.shortcuts.action.save_entry" },
      { combo: "Mod+N", descriptionKey: "workspace.information.shortcuts.action.new_entry" },
      { combo: "Mod+D", descriptionKey: "workspace.information.shortcuts.action.delete_entry" },
      { combo: "Mod+Z", descriptionKey: "workspace.information.shortcuts.action.undo_entry", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
      { combo: "Mod+Shift+Z", descriptionKey: "workspace.information.shortcuts.action.redo_entry", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
      { combo: "Mod+M", descriptionKey: "workspace.information.shortcuts.action.toggle_meta" },
      { combo: "Ctrl+Shift+M", descriptionKey: "workspace.information.shortcuts.action.preview_markdown" },
      { combo: "Ctrl+Shift+H", descriptionKey: "workspace.information.shortcuts.action.preview_html" },
      { combo: "ArrowUp", descriptionKey: "workspace.information.shortcuts.action.prev_entry", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
      { combo: "ArrowDown", descriptionKey: "workspace.information.shortcuts.action.next_entry", scopeKey: "workspace.information.shortcuts.scope.entry_focus_outside" },
    ],
  },
];
