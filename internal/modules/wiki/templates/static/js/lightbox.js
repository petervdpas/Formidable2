// Full-screen viewer for wiki content: click an image or a rendered mermaid
// diagram to open it in an overlay; click the backdrop, the close button, or
// press Esc to dismiss. Inside the overlay the item zooms (wheel) and pans
// (drag); double-click resets. Mermaid's inline SVG ships a viewBox but no
// width or height, so the clone is sized explicitly from the viewBox (scaled
// to fit the viewport) before any zoom transform is layered on top.
(function () {
  var main = document.querySelector("main.page-wrap") || document.body;

  var ZOOM_MIN = 1;
  var ZOOM_MAX = 8;
  var ZOOM_STEP = 1.15;

  var overlay = document.createElement("div");
  overlay.className = "lightbox-overlay";
  overlay.hidden = true;
  overlay.setAttribute("role", "dialog");
  overlay.setAttribute("aria-modal", "true");

  var closeBtn = document.createElement("button");
  closeBtn.type = "button";
  closeBtn.className = "lightbox-close";
  closeBtn.setAttribute("aria-label", "Close");
  closeBtn.textContent = "×";
  overlay.appendChild(closeBtn);

  document.body.appendChild(overlay);

  var item = null; // the img or svg currently shown
  var scale = 1;
  var tx = 0;
  var ty = 0;

  function clamp(z) {
    return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, z));
  }

  function applyTransform() {
    if (item) item.style.transform =
      "translate(" + tx + "px, " + ty + "px) scale(" + scale + ")";
  }

  function resetTransform() {
    scale = 1;
    tx = 0;
    ty = 0;
    applyTransform();
  }

  function fitSvg(svg) {
    var vb = svg.viewBox && svg.viewBox.baseVal;
    if (!vb || !vb.width || !vb.height) return;
    var k = Math.min(
      (window.innerWidth * 0.95) / vb.width,
      (window.innerHeight * 0.95) / vb.height,
    );
    svg.style.width = Math.round(vb.width * k) + "px";
    svg.style.height = Math.round(vb.height * k) + "px";
  }

  function clearContent() {
    var items = overlay.querySelectorAll(".lightbox-item");
    Array.prototype.forEach.call(items, function (n) {
      overlay.removeChild(n);
    });
    item = null;
  }

  function show() {
    overlay.hidden = false;
    document.body.style.overflow = "hidden";
  }

  function hide() {
    overlay.hidden = true;
    document.body.style.overflow = "";
    clearContent();
  }

  function wireItem(el) {
    item = el;
    el.classList.add("lightbox-item");
    resetTransform();
    el.addEventListener("click", stop);
    el.addEventListener("dblclick", function (e) {
      stop(e);
      resetTransform();
    });
    el.addEventListener("pointerdown", onPointerDown);
    overlay.appendChild(el);
    show();
  }

  function openImage(src, alt) {
    clearContent();
    var img = document.createElement("img");
    img.src = src;
    if (alt) img.alt = alt;
    wireItem(img);
  }

  function openSvg(svg) {
    clearContent();
    var clone = svg.cloneNode(true);
    wireItem(clone);
    fitSvg(clone);
  }

  function stop(e) {
    e.stopPropagation();
  }

  // Drag-pan. Pointer events start on the item, move/end on the document so a
  // fast drag that leaves the item still pans.
  var dragging = false;
  var px = 0;
  var py = 0;

  function onPointerDown(e) {
    if (e.button !== 0) return;
    e.stopPropagation();
    e.preventDefault();
    dragging = true;
    px = e.clientX;
    py = e.clientY;
    if (item) item.classList.add("is-grabbing");
  }

  function onPointerMove(e) {
    if (!dragging) return;
    tx += e.clientX - px;
    ty += e.clientY - py;
    px = e.clientX;
    py = e.clientY;
    applyTransform();
  }

  function onPointerUp() {
    dragging = false;
    if (item) item.classList.remove("is-grabbing");
  }

  main.addEventListener("click", function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    var img = t.closest("img");
    if (img && main.contains(img)) {
      e.preventDefault();
      openImage(img.currentSrc || img.src, img.alt);
      return;
    }
    var svg = t.closest("pre.mermaid svg");
    if (svg && main.contains(svg)) {
      e.preventDefault();
      openSvg(svg);
    }
  });

  overlay.addEventListener("click", function () {
    if (!dragging) hide();
  });
  overlay.addEventListener(
    "wheel",
    function (e) {
      if (!item) return;
      e.preventDefault();
      scale = clamp(scale * (e.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP));
      applyTransform();
    },
    { passive: false },
  );

  closeBtn.addEventListener("click", function (e) {
    stop(e);
    hide();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && !overlay.hidden) hide();
  });
  document.addEventListener("pointermove", onPointerMove);
  document.addEventListener("pointerup", onPointerUp);
  window.addEventListener("resize", function () {
    if (item && item.tagName.toLowerCase() === "svg") fitSvg(item);
  });
})();
