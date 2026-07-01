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
  const read = (key: string, def: number) => {
    for (const opt of field.options ?? []) {
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
  return { w: read("canvas_width", SLIDE_CANVAS_W), h: read("canvas_height", SLIDE_CANVAS_H) };
}

// parseSlideDoc coerces a stored value into a SlideDoc; anything unexpected is
// an empty doc so the editor always has a blocks array to work with.
export function parseSlideDoc(v: unknown): SlideDoc {
  if (v && typeof v === "object" && Array.isArray((v as { blocks?: unknown }).blocks)) {
    return { blocks: (v as { blocks: SlideBlock[] }).blocks };
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

// tableColumns derives string columns from a table block's data width so the
// FormFieldTable editor has columns to render without the block storing its own
// column config (min 2).
function tableColumns(content: unknown): unknown[] {
  let cols = 2;
  if (Array.isArray(content)) {
    for (const row of content) {
      if (Array.isArray(row)) cols = Math.max(cols, row.length);
    }
  }
  return Array.from({ length: cols }, (_, i) => ({
    value: `c${i}`,
    label: `Col ${i + 1}`,
    type: "string",
  }));
}

// Reveal element kinds whose content is edited by an existing field component
// (the genuine reuse: a table IS a table). The rest (video, embed, code) get
// bespoke inspector editors and return null here.
const KIND_FIELD_TYPE: Record<string, string> = {
  text: "textarea",
  quote: "textarea",
  math: "textarea",
  image: "image",
  table: "table",
  list: "list",
  mermaid: "mermaid",
};

// fieldForBlock builds the synthetic Field a reuse-kind block binds to, or null
// for kinds with a bespoke editor.
export function fieldForBlock(b: SlideBlock): Field | null {
  const ft = KIND_FIELD_TYPE[b.kind];
  if (!ft) return null;
  const base: Record<string, unknown> = {
    key: b.id,
    type: ft,
    label: "",
    options: [],
    readonly: false,
  };
  if (b.kind === "text" || b.kind === "quote") base.format = "markdown";
  if (ft === "table") base.options = tableColumns(b.content);
  return base as unknown as Field;
}

// blockSummary is the short label shown on a block box in the canvas (the full
// content is edited in the inspector).
export function blockSummary(b: SlideBlock): string {
  switch (b.kind) {
    case "image":
      return typeof b.content === "string" && b.content ? b.content : "(no image)";
    case "table":
      return Array.isArray(b.content) ? `${b.content.length} rows` : "table";
    case "list":
      return Array.isArray(b.content) ? `${b.content.length} items` : "list";
    default: {
      const s = typeof b.content === "string" ? b.content.trim() : "";
      const first = s.split("\n")[0] ?? "";
      return first.length > 48 ? first.slice(0, 48) + "…" : first || "(empty)";
    }
  }
}
