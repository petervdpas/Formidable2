// Keyboard-combo helpers used by the topbar menu system. Combos are
// written cross-platform with the `Mod` token: Mod = ⌘ on macOS,
// Ctrl elsewhere. Multi-modifier combos use `+` as a separator and
// are case-insensitive. The single key (letter, digit, F-key, etc.)
// is the last segment.
//
//   "Mod+S"            →  Cmd+S on mac, Ctrl+S elsewhere
//   "Shift+Mod+S"      →  ⇧⌘S on mac, Ctrl+Shift+S elsewhere
//   "Alt+ArrowRight"   →  Alt+→ on both
//
// All shortcuts route through here so the rendered hint in MenuItem
// and the runtime matcher in useTopbarMenu agree byte-for-byte.

export interface ParsedCombo {
  ctrl: boolean;
  meta: boolean;
  alt: boolean;
  shift: boolean;
  /** Lowercased key value matched against `KeyboardEvent.key`. */
  key: string;
}

const isMac = /mac|iphone|ipad|ipod/i.test(
  typeof navigator !== "undefined" ? navigator.platform : "",
);

/** The label for the `Mod` token on the current platform - `⌘` on
 * macOS, `Ctrl` elsewhere. Use this in intro/help text so we don't
 * have to write "Cmd on macOS and Ctrl elsewhere" everywhere. */
export const modifierLabel: string = isMac ? "⌘" : "Ctrl";

export function parseCombo(combo: string): ParsedCombo | null {
  if (!combo) return null;
  const parts = combo.split("+").map((s) => s.trim()).filter(Boolean);
  if (parts.length === 0) return null;

  const out: ParsedCombo = {
    ctrl: false,
    meta: false,
    alt: false,
    shift: false,
    key: "",
  };

  for (const raw of parts) {
    const p = raw.toLowerCase();
    if (p === "mod") {
      if (isMac) out.meta = true;
      else out.ctrl = true;
    } else if (p === "ctrl" || p === "control") {
      out.ctrl = true;
    } else if (p === "cmd" || p === "command" || p === "meta") {
      out.meta = true;
    } else if (p === "alt" || p === "option") {
      out.alt = true;
    } else if (p === "shift") {
      out.shift = true;
    } else {
      // Last segment wins as the key; we don't error on extras because
      // the menu definition is checked at review-time, not runtime.
      out.key = p;
    }
  }
  return out.key ? out : null;
}

/** Render a combo for display next to a menu item. */
export function comboLabel(combo: string): string {
  const parsed = parseCombo(combo);
  if (!parsed) return combo;
  const tokens: string[] = [];
  if (isMac) {
    if (parsed.ctrl) tokens.push("⌃");
    if (parsed.alt) tokens.push("⌥");
    if (parsed.shift) tokens.push("⇧");
    if (parsed.meta) tokens.push("⌘");
    tokens.push(keyDisplay(parsed.key, true));
    return tokens.join("");
  }
  if (parsed.ctrl) tokens.push("Ctrl");
  if (parsed.alt) tokens.push("Alt");
  if (parsed.shift) tokens.push("Shift");
  if (parsed.meta) tokens.push("Meta");
  tokens.push(keyDisplay(parsed.key, false));
  return tokens.join("+");
}

function keyDisplay(key: string, mac: boolean): string {
  switch (key) {
    case "arrowleft":  return mac ? "←" : "Left";
    case "arrowright": return mac ? "→" : "Right";
    case "arrowup":    return mac ? "↑" : "Up";
    case "arrowdown":  return mac ? "↓" : "Down";
    case "escape":     return "Esc";
    case "enter":      return mac ? "↩" : "Enter";
    case "backspace":  return mac ? "⌫" : "Backspace";
    case "delete":     return mac ? "⌦" : "Del";
    case " ":          return "Space";
  }
  return key.length === 1 ? key.toUpperCase() : capitalize(key);
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** True if the keydown event matches the parsed combo. */
export function matchesCombo(e: KeyboardEvent, parsed: ParsedCombo): boolean {
  if (!!e.ctrlKey !== parsed.ctrl) return false;
  if (!!e.metaKey !== parsed.meta) return false;
  if (!!e.altKey !== parsed.alt) return false;
  if (!!e.shiftKey !== parsed.shift) return false;
  return (e.key || "").toLowerCase() === parsed.key;
}

/** True when the focused element would normally consume key events. */
export function isTypingTarget(el: EventTarget | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false;
  const tag = el.tagName.toLowerCase();
  if (tag === "input" || tag === "textarea") return true;
  if (el.isContentEditable) return true;
  return false;
}
