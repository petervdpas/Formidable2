// Slide block helpers. The block-kind palette is backend-owned
// (template.SlideBlockKinds); this module caches it and provides the small
// glue the canvas editor needs: parsing the stored doc, seeding new blocks,
// and building the synthetic Field a block's content editor binds to.

import { ref, type Ref } from "vue";
import {
  Service as TemplateSvc,
  type SlideBlockKindDescriptor,
  type Field,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

export interface SlideBlock {
  id: string;
  kind: string; // reveal element type
  content: unknown;
  x: number;
  y: number;
  w: number;
  h: number;
  fragment?: string; // reveal fragment animation ("" / undefined = none)
  lang?: string; // code block language
  style?: Record<string, string>; // per-element inline CSS (font-size, color, …)
}

export interface SlideDoc {
  blocks: SlideBlock[];
  background?: string; // reveal per-slide background (color)
  transition?: string; // reveal per-slide transition override
  notes?: string; // reveal speaker notes
}

// The fixed authoring stage (matches the Go SlideCanvasWidth/Height).
export const SLIDE_CANVAS_W = 1280;
export const SLIDE_CANVAS_H = 720;

const kinds: Ref<SlideBlockKindDescriptor[]> = ref([]);
let loadPromise: Promise<void> | null = null;

// ensureSlideBlockKindsLoaded fetches the backend palette once.
export function ensureSlideBlockKindsLoaded(): Promise<void> {
  if (!loadPromise) {
    loadPromise = TemplateSvc.SlideBlockKinds().then((k) => {
      kinds.value = k ?? [];
    });
  }
  return loadPromise;
}

export function slideBlockKinds(): SlideBlockKindDescriptor[] {
  return kinds.value;
}

// canvasSize reads the deck's authored canvas size from the slide field's
// options (deck-wide config), defaulting to 1280x720. Mirrors the Go
// SlideCanvasSize.
export function canvasSize(field: Field): { w: number; h: number } {
  const opts = field.options ?? [];
  // Any option whose label carries two integers is a format ("1920 x 1080
  // (16:9)"), wherever it sits - a clean canvas_format row, or (from an earlier
  // migration glitch) the legacy canvas_width row. Mirrors Go SlideCanvasSize.
  for (const opt of opts) {
    if (opt && typeof opt === "object") {
      const nums = String((opt as Record<string, unknown>).label ?? "").match(/\d+/g);
      if (nums && nums.length >= 2) {
        const w = parseInt(nums[0], 10), h = parseInt(nums[1], 10);
        if (w > 0 && h > 0) return { w, h };
      }
    }
  }
  // Genuinely old templates: separate canvas_width/canvas_height rows.
  const num = (key: string, def: number) => {
    for (const opt of opts) {
      if (opt && typeof opt === "object") {
        const o = opt as Record<string, unknown>;
        if (String(o.value ?? "") === key) {
          const n = parseInt(String(o.label ?? ""), 10);
          if (Number.isFinite(n) && n > 0) return n;
        }
      }
    }
    return def;
  };
  return { w: num("canvas_width", SLIDE_CANVAS_W), h: num("canvas_height", SLIDE_CANVAS_H) };
}

// parseSlideDoc coerces a stored value into a SlideDoc; anything unexpected is
// an empty doc so the editor always has a blocks array to work with. The
// slide-level attrs (background/transition/notes) round-trip too, so committing
// them does not wipe them on the next re-parse.
export function parseSlideDoc(v: unknown): SlideDoc {
  if (v && typeof v === "object" && Array.isArray((v as { blocks?: unknown }).blocks)) {
    const o = v as Record<string, unknown>;
    const doc: SlideDoc = { blocks: o.blocks as SlideBlock[] };
    if (typeof o.background === "string") doc.background = o.background;
    if (typeof o.transition === "string") doc.transition = o.transition;
    if (typeof o.notes === "string") doc.notes = o.notes;
    return doc;
  }
  return { blocks: [] };
}

function defaultContent(kind: string): unknown {
  switch (kind) {
    case "table":
    case "list":
      return [];
    default:
      return "";
  }
}

// newBlock seeds a fresh block, staggering position so successive adds don't
// land exactly on top of each other.
export function newBlock(kind: string, index: number): SlideBlock {
  const off = (index % 8) * 24;
  return {
    id: crypto.randomUUID(),
    kind,
    content: defaultContent(kind),
    x: 80 + off,
    y: 80 + off,
    w: 480,
    h: 240,
  };
}

// Kinds edited by typing directly on the slide (contenteditable-style), the way
// slides.com works. Their content is plain text (markdown / latex / code). The
// editor uses this to auto-enter inline edit on add and to gate double-click.
export const INLINE_TEXT_KINDS = new Set(["text", "quote", "math", "code"]);

// syntheticField builds the throwaway Field a reuse-kind slide component binds
// its existing FormField* editor to (image/list/mermaid). The block id is the
// key so nested editors stay isolated.
export function syntheticField(
  id: string,
  type: string,
  extra: Record<string, unknown> = {},
): Field {
  return { key: id, type, label: "", options: [], readonly: false, ...extra } as unknown as Field;
}
