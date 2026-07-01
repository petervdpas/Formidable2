// Wiki deck bootstrap: the non-Vue equivalent of RevealDeck.vue. Boots reveal.js
// over the server-rendered .reveal element and hydrates the same lazy content the
// editor does - KaTeX math (<pre class="katex-math">) and mermaid diagrams
// (<pre class="mermaid">). Loaded after reveal.js, katex.min.js and mermaid.min.js.
(function () {
  var el = document.querySelector(".reveal");
  if (!el || typeof Reveal === "undefined") {
    console.error("[formidable] deck: reveal.js failed to load");
    return;
  }
  var width = parseInt(el.getAttribute("data-width"), 10) || 1280;
  var height = parseInt(el.getAttribute("data-height"), 10) || 720;

  // mermaid.min.js exposes the API as __esbuild_esm_mermaid_nm.mermaid (not
  // window.mermaid); mirror the wiki's mermaid-init.js detection.
  function getMermaid() {
    if (window.mermaid && window.mermaid.run) return window.mermaid;
    var ns = window.__esbuild_esm_mermaid_nm;
    return ns && ns.mermaid && ns.mermaid.run ? ns.mermaid : null;
  }
  var mermaidReady = false;

  // Hydrate KaTeX: each <pre class="katex-math"> holds raw LaTeX; render it in
  // place. Idempotent via data-tex so re-entry on slidechanged is a no-op.
  function hydrateKatex(scope) {
    if (typeof katex === "undefined" || !scope) return;
    var nodes = scope.querySelectorAll(".katex-math");
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (n.dataset.tex !== undefined) continue;
      var src = n.textContent || "";
      n.dataset.tex = src;
      try {
        katex.render(src, n, { throwOnError: false, displayMode: true });
      } catch (e) {
        /* keep the raw source on failure */
      }
    }
  }

  // Hydrate mermaid on the CURRENT slide only: a diagram on a display:none slide
  // can't be measured and renders broken, so run per-slide as each becomes active.
  function hydrateMermaid(scope) {
    if (!scope) return;
    var m = getMermaid();
    if (!m) return;
    var nodes = scope.querySelectorAll(".mermaid");
    if (!nodes.length) return;
    if (!mermaidReady) {
      m.initialize({ startOnLoad: false, securityLevel: "strict", theme: "default" });
      mermaidReady = true;
    }
    var todo = [];
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (n.getAttribute("data-processed")) continue;
      if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent || "";
      n.textContent = n.dataset.mmsrc;
      todo.push(n);
    }
    if (todo.length) {
      try {
        m.run({ nodes: todo });
      } catch (e) {
        /* parse error: keep source text */
      }
    }
  }

  function hydrate(scope) {
    var s = scope || el;
    hydrateKatex(s);
    hydrateMermaid(s);
  }

  var deck = new Reveal(el, {
    embedded: false,
    width: width,
    height: height,
    margin: 0,
    center: false,
    controls: true,
    progress: true,
    hash: true,
  });
  deck.initialize().then(function () {
    hydrate(deck.getCurrentSlide());
  });
  deck.on("slidechanged", function (ev) {
    hydrate(ev.currentSlide);
  });
})();
