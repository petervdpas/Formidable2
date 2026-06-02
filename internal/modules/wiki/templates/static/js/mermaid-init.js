// Hydrate ```mermaid fences (rendered as <pre class="mermaid"> by the
// goldmark client-mode extension) into diagrams. Loaded after
// mermaid.min.js, which exposes the API as __esbuild_esm_mermaid_nm.mermaid
// (not window.mermaid).
(function () {
  function getMermaid() {
    if (window.mermaid && window.mermaid.run) return window.mermaid;
    var ns = window.__esbuild_esm_mermaid_nm;
    return ns && ns.mermaid && ns.mermaid.run ? ns.mermaid : null;
  }
  var m = getMermaid();
  if (!m) {
    console.error("[formidable] mermaid failed to load");
    return;
  }
  var nodes = document.querySelectorAll(".mermaid");
  if (!nodes.length) return;
  m.initialize({ startOnLoad: false, securityLevel: "strict", theme: "default" });
  nodes.forEach(function (n) {
    if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent || "";
    n.removeAttribute("data-processed");
    n.textContent = n.dataset.mmsrc;
  });
  m.run({ querySelector: ".mermaid" });
})();
