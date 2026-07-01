// Single source of truth for turning `<pre class="mermaid">` blocks into
// self-contained SVG diagrams. Every Vue surface (RenderedHtml, RevealDeck) uses
// this; the wiki's plain-JS deck-init.js mirrors it byte-for-byte in logic.
//
// Two isolation guarantees, both deliberate:
//  1. mermaid.render() (NOT run()) builds each diagram in mermaid's own off-DOM
//     container, so text is measured in a clean, untransformed context. run()
//     renders in place and mis-measures inside a scaled/perspective-transformed
//     host (reveal slides, the editor's scaled stage), clipping node text.
//  2. The produced SVG is pinned to mermaid's own font ON THE SVG ELEMENT, so the
//     diagram is a portable artifact: no host page CSS (formidable-prose, a video
//     block on the same slide, the wiki chrome) can shift the text off the boxes
//     it was measured against. mermaid emits `font-family: var(--mermaid-font-
//     family)` and defines that variable via a <style> that does NOT travel with
//     the transplanted SVG; we redefine it on the SVG root so it always resolves.

import type MermaidDefault from "mermaid";
type Mermaid = typeof MermaidDefault;

// mermaid's default-theme font stack; node boxes are measured against it, so the
// display must resolve to the same stack (all fall back to the system sans-serif
// on Linux, identically for measure and display, so the boxes fit).
export const MERMAID_FONT = '"trebuchet ms", verdana, arial, sans-serif';

let mermaidPromise: Promise<Mermaid> | null = null;
export function loadMermaid(): Promise<Mermaid> {
  if (!mermaidPromise) mermaidPromise = import("mermaid").then((m) => m.default);
  return mermaidPromise;
}

let seq = 0;

// hydrateMermaid renders every `.mermaid` under root into a self-contained SVG.
// isCurrent lets a caller abort a superseded async run (theme/content change)
// so overlapping calls don't interleave DOM writes.
export async function hydrateMermaid(
  root: HTMLElement | null | undefined,
  theme: "default" | "dark",
  isCurrent: () => boolean = () => true,
): Promise<void> {
  if (!root) return;
  const nodes = Array.from(root.querySelectorAll<HTMLElement>(".mermaid"));
  if (nodes.length === 0) return;
  const mermaid = await loadMermaid();
  if (!isCurrent()) return;
  mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme });
  // Measure against ready font metrics (a slide still loading a video iframe can
  // otherwise reflow mid-measure).
  await (document.fonts?.ready ?? Promise.resolve());
  for (const n of nodes) {
    if (!isCurrent()) return;
    if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent ?? "";
    const src = n.dataset.mmsrc;
    try {
      const { svg, bindFunctions } = await mermaid.render(`fmd-mermaid-${seq++}`, src);
      if (!isCurrent()) return;
      n.innerHTML = svg;
      n.querySelector<SVGElement>("svg")?.style.setProperty("--mermaid-font-family", MERMAID_FONT);
      n.setAttribute("data-processed", "true");
      bindFunctions?.(n);
    } catch {
      n.textContent = src; // parse error: keep the source text
    }
  }
}
