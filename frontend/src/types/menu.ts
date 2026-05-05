// MenuEntry is the data shape every workspace publishes for the topbar
// to render. There is one rule about shape: a top-level entry without
// children IS just a clickable action; one WITH children is a dropdown
// group. The renderer (MenuButton.vue) dispatches on that.

/** A clickable leaf action. May appear at top level or inside a group. */
export interface MenuAction {
  type?: "action";          // optional discriminator
  id: string;               // stable key for v-for / aria
  labelKey: string;         // i18n key for the visible label
  hintKey?: string;         // optional i18n key for a right-aligned hint (shortcut)
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
