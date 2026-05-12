// Curated character catalog for the Footer's CharPicker. Each entry
// carries a lowercase searchable `name` so the popover's filter input
// can match by typed words (e.g. "alpha", "right arrow", "euro")
// without depending on the Unicode database at runtime. The set is
// intentionally smaller than every glyph in a category — it's the
// shortlist the average user reaches for, not a Unicode browser.

export interface CharEntry {
  char: string;
  name: string;
}

export interface CharCategory {
  id: string;
  labelKey: string;
  items: CharEntry[];
}

export const CHAR_CATEGORIES: CharCategory[] = [
  {
    id: "arrows",
    labelKey: "charpicker.tab.arrows",
    items: [
      { char: "→", name: "right arrow" },
      { char: "←", name: "left arrow" },
      { char: "↑", name: "up arrow" },
      { char: "↓", name: "down arrow" },
      { char: "↔", name: "left right arrow" },
      { char: "↕", name: "up down arrow" },
      { char: "↖", name: "north west arrow" },
      { char: "↗", name: "north east arrow" },
      { char: "↘", name: "south east arrow" },
      { char: "↙", name: "south west arrow" },
      { char: "⇒", name: "right double arrow" },
      { char: "⇐", name: "left double arrow" },
      { char: "⇑", name: "up double arrow" },
      { char: "⇓", name: "down double arrow" },
      { char: "⇔", name: "left right double arrow" },
      { char: "⇕", name: "up down double arrow" },
    ],
  },
  {
    id: "greek",
    labelKey: "charpicker.tab.greek",
    items: [
      { char: "α", name: "alpha" },
      { char: "β", name: "beta" },
      { char: "γ", name: "gamma" },
      { char: "δ", name: "delta" },
      { char: "ε", name: "epsilon" },
      { char: "ζ", name: "zeta" },
      { char: "η", name: "eta" },
      { char: "θ", name: "theta" },
      { char: "ι", name: "iota" },
      { char: "κ", name: "kappa" },
      { char: "λ", name: "lambda" },
      { char: "μ", name: "mu" },
      { char: "ν", name: "nu" },
      { char: "ξ", name: "xi" },
      { char: "ο", name: "omicron" },
      { char: "π", name: "pi" },
      { char: "ρ", name: "rho" },
      { char: "σ", name: "sigma" },
      { char: "τ", name: "tau" },
      { char: "υ", name: "upsilon" },
      { char: "φ", name: "phi" },
      { char: "χ", name: "chi" },
      { char: "ψ", name: "psi" },
      { char: "ω", name: "omega" },
    ],
  },
  {
    id: "math",
    labelKey: "charpicker.tab.math",
    items: [
      { char: "±", name: "plus minus" },
      { char: "×", name: "times multiplication" },
      { char: "÷", name: "divide division" },
      { char: "√", name: "square root" },
      { char: "∞", name: "infinity" },
      { char: "≈", name: "approximately" },
      { char: "≠", name: "not equal" },
      { char: "≤", name: "less or equal" },
      { char: "≥", name: "greater or equal" },
      { char: "≡", name: "identical equivalent" },
      { char: "∝", name: "proportional" },
      { char: "∴", name: "therefore" },
      { char: "∂", name: "partial derivative" },
      { char: "∆", name: "delta increment" },
      { char: "∇", name: "nabla gradient" },
      { char: "Σ", name: "sigma sum" },
      { char: "Π", name: "pi product" },
      { char: "∫", name: "integral" },
      { char: "∈", name: "element of" },
      { char: "∉", name: "not element of" },
      { char: "⊂", name: "subset" },
      { char: "⊃", name: "superset" },
      { char: "∪", name: "union" },
      { char: "∩", name: "intersection" },
    ],
  },
  {
    id: "latin",
    labelKey: "charpicker.tab.latin",
    items: [
      { char: "à", name: "a grave" },
      { char: "á", name: "a acute" },
      { char: "â", name: "a circumflex" },
      { char: "ã", name: "a tilde" },
      { char: "ä", name: "a umlaut" },
      { char: "å", name: "a ring" },
      { char: "æ", name: "ae ligature" },
      { char: "ç", name: "c cedilla" },
      { char: "è", name: "e grave" },
      { char: "é", name: "e acute" },
      { char: "ê", name: "e circumflex" },
      { char: "ë", name: "e umlaut" },
      { char: "ì", name: "i grave" },
      { char: "í", name: "i acute" },
      { char: "î", name: "i circumflex" },
      { char: "ï", name: "i umlaut" },
      { char: "ñ", name: "n tilde" },
      { char: "ò", name: "o grave" },
      { char: "ó", name: "o acute" },
      { char: "ô", name: "o circumflex" },
      { char: "õ", name: "o tilde" },
      { char: "ö", name: "o umlaut" },
      { char: "ø", name: "o stroke slash" },
      { char: "ù", name: "u grave" },
      { char: "ú", name: "u acute" },
      { char: "û", name: "u circumflex" },
      { char: "ü", name: "u umlaut" },
      { char: "ý", name: "y acute" },
      { char: "ÿ", name: "y umlaut" },
      { char: "ð", name: "eth" },
      { char: "þ", name: "thorn" },
      { char: "ß", name: "sharp s eszett" },
      { char: "œ", name: "oe ligature" },
      { char: "ł", name: "l stroke slash" },
    ],
  },
  {
    id: "symbols",
    labelKey: "charpicker.tab.symbols",
    items: [
      { char: "©", name: "copyright" },
      { char: "®", name: "registered" },
      { char: "™", name: "trademark" },
      { char: "§", name: "section" },
      { char: "¶", name: "pilcrow paragraph" },
      { char: "†", name: "dagger" },
      { char: "‡", name: "double dagger" },
      { char: "•", name: "bullet" },
      { char: "·", name: "middle dot" },
      { char: "…", name: "ellipsis" },
      { char: "—", name: "em dash" },
      { char: "–", name: "en dash" },
      { char: "★", name: "star filled" },
      { char: "☆", name: "star outline" },
      { char: "✓", name: "check tick" },
      { char: "✗", name: "cross x" },
      { char: "⚠", name: "warning" },
      { char: "♠", name: "spade" },
      { char: "♥", name: "heart" },
      { char: "♦", name: "diamond" },
      { char: "♣", name: "club" },
      { char: "“", name: "left double quote" },
      { char: "”", name: "right double quote" },
      { char: "‘", name: "left single quote" },
      { char: "’", name: "right single quote apostrophe" },
    ],
  },
  {
    id: "currency",
    labelKey: "charpicker.tab.currency",
    items: [
      { char: "€", name: "euro" },
      { char: "£", name: "pound sterling" },
      { char: "¥", name: "yen yuan" },
      { char: "¢", name: "cent" },
      { char: "₹", name: "rupee" },
      { char: "₽", name: "ruble" },
      { char: "₩", name: "won" },
      { char: "₪", name: "shekel" },
      { char: "₿", name: "bitcoin" },
      { char: "¤", name: "currency sign generic" },
    ],
  },
];

/** Render U+XXXX (or U+XXXXX) for the hover tooltip. */
export function codepoint(ch: string): string {
  const cp = ch.codePointAt(0);
  if (cp == null) return "";
  return "U+" + cp.toString(16).toUpperCase().padStart(4, "0");
}
