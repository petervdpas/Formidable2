// Reading zoom for the wiki content.
//
// The wiki is a browsing surface of its own: a peer on another machine may read
// it, and an exported bundle is a folder of files with no backend behind it. So
// the level lives in the reader's own browser rather than in a Formidable
// profile, and the level set is here rather than fetched. Only the page body
// scales; the topbar keeps its size so the controls stay where the eye left
// them.
(function () {
  "use strict";

  var LEVELS = [75, 90, 100, 110, 125, 150, 175, 200];
  var DEFAULT = 100;
  var KEY = "formidable.wiki.zoom";

  function read() {
    try {
      var pct = parseInt(window.localStorage.getItem(KEY), 10);
      return LEVELS.indexOf(pct) >= 0 ? pct : DEFAULT;
    } catch (e) {
      // Private mode, or storage denied: the default still reads fine.
      return DEFAULT;
    }
  }

  function write(pct) {
    try {
      window.localStorage.setItem(KEY, String(pct));
    } catch (e) {
      // Not persisting is survivable; the page is already zoomed.
    }
  }

  function apply(pct) {
    var main = document.querySelector("main.page-wrap");
    if (main) main.style.zoom = pct / 100;
  }

  function init() {
    var pct = read();
    apply(pct);

    var select = document.getElementById("zoom");
    if (!select) return;
    LEVELS.forEach(function (level) {
      var opt = document.createElement("option");
      opt.value = String(level);
      opt.textContent = level + "%";
      if (level === pct) opt.selected = true;
      select.appendChild(opt);
    });
    select.addEventListener("change", function () {
      var next = parseInt(select.value, 10);
      if (LEVELS.indexOf(next) < 0) return;
      apply(next);
      write(next);
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
