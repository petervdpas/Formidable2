// Shared KaTeX hydration for slide math blocks. The backend emits math as
// `<pre class="katex-math">LATEX</pre>` (a hydratable element like pre.mermaid);
// every surface that shows that HTML (the editor's RenderedHtml, the deck's
// RevealDeck) calls hydrateKatex to render it in place. KaTeX is lazy-imported
// so it only loads when a deck/slide actually uses math.
import "katex/dist/katex.min.css";

type KatexAPI = (typeof import("katex"))["default"];
let katexPromise: Promise<KatexAPI> | null = null;
function loadKatex() {
  if (!katexPromise) katexPromise = import("katex").then((m) => m.default);
  return katexPromise;
}

export async function hydrateKatex(root: HTMLElement | null): Promise<void> {
  if (!root) return;
  const nodes = Array.from(root.querySelectorAll<HTMLElement>(".katex-math"));
  if (nodes.length === 0) return;
  const katex = await loadKatex();
  for (const n of nodes) {
    // Stash the source once so re-runs render from LaTeX, not the injected SVG.
    if (n.dataset.tex === undefined) n.dataset.tex = n.textContent ?? "";
    const src = n.dataset.tex;
    if (!src.trim()) continue;
    try {
      katex.render(src, n, { throwOnError: false, displayMode: true });
    } catch {
      /* invalid LaTeX: leave the source text as-is */
    }
  }
}
