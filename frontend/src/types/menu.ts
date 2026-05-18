// MenuEntry is the data shape every workspace publishes for the topbar
// to render. There is one rule about shape: a top-level entry without
// children IS just a clickable action; one WITH children is a dropdown
// group. The renderer (MenuButton.vue) dispatches on that.

/** A clickable leaf action. May appear at top level or inside a group. */
export interface MenuAction {
  type?: "action";          // optional discriminator
  id: string;               // stable key for v-for / aria
  /**
   * i18n key for the visible label. Required so static menu entries
   * stay translation-driven. Dynamic, user-authored labels (plugin
   * commands, repo names, etc.) pass `label` instead and may use any
   * placeholder for `labelKey`; `label` always wins when present.
   */
  labelKey: string;
  /** Literal label that supersedes labelKey — for runtime-named items. */
  label?: string;
  hintKey?: string;         // optional i18n key for a right-aligned hint
  /**
   * Optional cross-platform keyboard shortcut bound globally for as long
   * as this menu is registered. Written with the `Mod` token:
   * `"Mod+S"` (Cmd+S on macOS, Ctrl+S elsewhere), `"Shift+Mod+S"`, etc.
   * Rendered as the right-aligned hint in the menu item — supersedes
   * `hintKey` when present.
   */
  combo?: string;
  /**
   * Default: false. When false, the shortcut is suppressed while focus
   * is in an input / textarea / contenteditable so editor shortcuts
   * (Ctrl+S inside CodeMirror) aren't hijacked. Set true to fire
   * regardless of focus.
   */
  allowWhenTyping?: boolean;
  disabled?: boolean;
  onClick: () => void | Promise<void>;
}

/** A dropdown that opens a popup when clicked. */
export interface MenuGroup {
  type: "group";
  id: string;
  labelKey: string;
  items: Array<MenuAction | MenuSeparator>;
  /**
   * Bypass the "auto-disable when every child is disabled" rule. Use
   * for groups that should stay clickable for discoverability — e.g.
   * a File menu where seeing greyed-out items helps the user understand
   * what operations exist, even if none currently apply.
   */
  alwaysEnabled?: boolean;
}

/** A horizontal divider inside a group. */
export interface MenuSeparator {
  type: "separator";
  id?: string;
}

/** What workspaces register at the top level of the topbar. */
export type MenuEntry = MenuAction | MenuGroup;
