// Standalone document hydration: the server-less counterpart of the wiki's
// mermaid-init.js. Renders mermaid diagrams (<pre class="mermaid">) and KaTeX
// math (<pre class="katex-math">) in an exported single-file page. Loaded after
// mermaid.min.js / katex.min.js, each inlined only when its content is present.
(function () {
  function getMermaid() {
    if (window.mermaid && window.mermaid.run) return window.mermaid;
    var ns = window.__esbuild_esm_mermaid_nm;
    return ns && ns.mermaid && ns.mermaid.run ? ns.mermaid : null;
  }
  var m = getMermaid();
  if (m) {
    var diagrams = document.querySelectorAll(".mermaid");
    if (diagrams.length) {
      m.initialize({ startOnLoad: false, securityLevel: "strict", theme: "default" });
      diagrams.forEach(function (n) {
        if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent || "";
        n.removeAttribute("data-processed");
        n.textContent = n.dataset.mmsrc;
      });
      m.run({ querySelector: ".mermaid" });
    }
  }
  if (typeof katex !== "undefined") {
    var math = document.querySelectorAll(".katex-math");
    for (var i = 0; i < math.length; i++) {
      var n = math[i];
      if (n.dataset.tex !== undefined) continue;
      var src = n.textContent || "";
      n.dataset.tex = src;
      try {
        katex.render(src, n, { throwOnError: false, displayMode: true });
      } catch (e) {
        /* parse error: keep the raw source */
      }
    }
  }
})();
