// downloadChartSvg exports a rendered chart DOM node as a self-contained
// .svg file. The StatGrid charts are a mix of inline SVG (bars, pie
// slices) and HTML (the pie legend, heatmap table, scalar cards), and
// their colors/fonts come from external stylesheets (incl. the facet
// option colors via currentColor). To produce a file that renders the
// same anywhere, we:
//   1. clone the node,
//   2. inline the *computed* style of every element (so facet colors,
//      currentColor, fonts and layout are baked in as concrete values),
//   3. wrap the clone in an <svg><foreignObject> at the node's pixel
//      size, with an opaque background.
// Pure-frontend, no canvas rasterization - so it's reliable on WebKit
// (where SVG->canvas often taints or blanks). The output opens in any
// browser.

// Curated set of style properties worth baking in. Copying the full
// computed style bloats the file and serializes shorthands oddly; this
// covers colors, type, and the flex/grid layout the charts use.
const STYLE_PROPS = [
  "color",
  "background-color",
  "fill",
  "fill-opacity",
  "stroke",
  "stroke-width",
  "opacity",
  "font-family",
  "font-size",
  "font-weight",
  "font-style",
  "text-anchor",
  "text-align",
  "line-height",
  "white-space",
  "letter-spacing",
  "display",
  "flex-direction",
  "align-items",
  "justify-content",
  "gap",
  "padding",
  "margin",
  "border",
  "border-radius",
  "box-sizing",
  "width",
  "height",
  "transform",
] as const;

function inlineComputedStyles(src: Element, dst: Element): void {
  const cs = window.getComputedStyle(src);
  let css = "";
  for (const prop of STYLE_PROPS) {
    const v = cs.getPropertyValue(prop);
    if (v) css += `${prop}:${v};`;
  }
  // dst is HTML or SVG; both expose .style.
  (dst as HTMLElement | SVGElement).setAttribute(
    "style",
    ((dst as HTMLElement).getAttribute("style") ?? "") + css,
  );
  const s = src.children;
  const d = dst.children;
  for (let i = 0; i < s.length && i < d.length; i++) {
    inlineComputedStyles(s[i], d[i]);
  }
}

const SVG_NS = "http://www.w3.org/2000/svg";

function surfaceColor(node: HTMLElement): string {
  // Walk up for the first opaque background so the export isn't
  // transparent; fall back to the app's surface token, then white.
  let el: HTMLElement | null = node;
  while (el) {
    const bg = window.getComputedStyle(el).backgroundColor;
    if (bg && bg !== "transparent" && !bg.startsWith("rgba(0, 0, 0, 0")) return bg;
    el = el.parentElement;
  }
  const tok = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue("--color-surface")
    .trim();
  return tok || "#ffffff";
}

// serializeSvg exports a single self-contained <svg> with its computed
// styles inlined (facet colors / fonts baked) and an opaque background.
// Renders the same in any viewer (browser, VS Code, Inkscape) - no
// foreignObject.
function serializeSvg(svg: SVGSVGElement, bg: string): string {
  const clone = svg.cloneNode(true) as SVGSVGElement;
  inlineComputedStyles(svg, clone);
  clone.setAttribute("xmlns", SVG_NS);

  let w = 0;
  let h = 0;
  const vb = svg.getAttribute("viewBox");
  if (vb) {
    const p = vb.split(/[\s,]+/).map(Number);
    w = p[2];
    h = p[3];
  }
  if (!w || !h) {
    const r = svg.getBoundingClientRect();
    w = Math.ceil(r.width);
    h = Math.ceil(r.height);
  }
  clone.setAttribute("width", String(w));
  clone.setAttribute("height", String(h));

  const rect = document.createElementNS(SVG_NS, "rect");
  rect.setAttribute("width", "100%");
  rect.setAttribute("height", "100%");
  rect.setAttribute("fill", bg);
  clone.insertBefore(rect, clone.firstChild);

  return `<?xml version="1.0" encoding="UTF-8"?>\n${new XMLSerializer().serializeToString(clone)}`;
}

// buildChartSvg returns a self-contained SVG document string for the
// chart node. The caller writes it to disk through the backend
// (Dialog.ChooseSaveFile + System.SaveFile) - the webview doesn't honor
// browser <a download> blob saves on WebKitGTK, so file writes go
// through Go like every other export.
//
// When the chart is a single inline <svg> (bar, pie - both render their
// own legend in SVG), we export that SVG directly so it shows in every
// viewer. HTML-only charts (heatmap table, scalar cards) fall back to a
// foreignObject wrap, which only renders in a browser.
export function buildChartSvg(node: HTMLElement): string {
  const bg = surfaceColor(node);
  const svgs = node.querySelectorAll("svg");
  if (svgs.length === 1) {
    return serializeSvg(svgs[0] as SVGSVGElement, bg);
  }

  const rect = node.getBoundingClientRect();
  const w = Math.max(1, Math.ceil(rect.width));
  const h = Math.max(1, Math.ceil(rect.height));
  const clone = node.cloneNode(true) as HTMLElement;
  inlineComputedStyles(node, clone);
  const inner = new XMLSerializer().serializeToString(clone);
  return (
    `<svg xmlns="${SVG_NS}" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">` +
    `<rect width="100%" height="100%" fill="${bg}"/>` +
    `<foreignObject x="0" y="0" width="${w}" height="${h}">` +
    `<div xmlns="http://www.w3.org/1999/xhtml">${inner}</div>` +
    `</foreignObject></svg>`
  );
}
