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
  // place. Idempotent via data-tex so re-hydration is a no-op.
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

  // Mirror of frontend/src/utils/mermaidHydrate.ts (kept in lock-step; the wiki is
  // a separate plain-JS runtime so it can't import the TS module). Two isolation
  // guarantees: (1) mermaid.render() builds each diagram in a clean off-DOM
  // container so text is measured right regardless of reveal's scaled/perspective
  // slide; (2) the produced SVG is pinned to mermaid's own font ON THE SVG, so no
  // host CSS (formidable-prose, a video block on the same slide) can shift the
  // text off its measured boxes.
  var MERMAID_FONT = '"trebuchet ms", verdana, arial, sans-serif';
  var mermaidSeq = 0;
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
    var fontsReady = document.fonts && document.fonts.ready ? document.fonts.ready : Promise.resolve();
    fontsReady.then(function () {
      for (var i = 0; i < nodes.length; i++) {
        (function (n) {
          if (n.getAttribute("data-processed")) return;
          if (n.dataset.mmsrc === undefined) n.dataset.mmsrc = n.textContent || "";
          var src = n.dataset.mmsrc;
          n.setAttribute("data-processed", "true");
          m.render("fmd-mermaid-" + mermaidSeq++, src)
            .then(function (out) {
              n.innerHTML = out.svg;
              var svg = n.querySelector("svg");
              if (svg) svg.style.setProperty("--mermaid-font-family", MERMAID_FONT);
              if (out.bindFunctions) out.bindFunctions(n);
            })
            .catch(function () {
              n.removeAttribute("data-processed");
              n.textContent = src; // parse error: keep source
            });
        })(nodes[i]);
      }
    });
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
    // mermaid.render builds diagrams off-DOM, so hydrate every slide up front:
    // overview (ESC) then shows real content, not raw code blocks.
    hydrate(el);
  });
})();
