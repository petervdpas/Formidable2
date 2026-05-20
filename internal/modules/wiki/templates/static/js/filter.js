// Wiki list filtering — tags + free-text + facets, all client-side.
//
//   * #tag / plain text live filter (the existing topbar search box).
//   * Per-facet <select> dropdowns in the .facet-filter-strip above
//     the form list. AND-combined with the text filter and with each
//     other: every facet that has a non-empty value must match.
//
// Filters degrade gracefully — pages without the search input still
// work (no listeners attached), pages without facet selects work too.
(function () {
  const items = Array.from(document.querySelectorAll('.form-picker-list [data-tags]'));
  const q = document.getElementById('q');
  const facetSelects = Array.from(document.querySelectorAll('.facet-filter-select'));
  const clearBtn = document.querySelector('[data-facet-filter-clear]');

  // Tag/text search is only enabled when at least one row carries tags.
  const hasTags = items.some(li => (li.getAttribute('data-tags') || '').trim().length > 0);
  if (q) {
    if (hasTags) {
      q.disabled = false;
    } else {
      q.disabled = true;
      q.placeholder = 'No #tags available on this page';
    }
  }

  // Paint the selected option's colour onto the <select> trigger so
  // the active facet is visible without opening the dropdown. Empty
  // value falls back to the neutral chip look.
  function paintSelect(sel) {
    const opt = sel.options[sel.selectedIndex];
    const color = (opt && opt.getAttribute('data-facet-color')) || '';
    if (color) {
      sel.setAttribute('data-facet-active-color', color);
    } else {
      sel.removeAttribute('data-facet-active-color');
    }
  }

  function readFacetFilters() {
    // { key: label } for every non-empty <select>.
    const out = {};
    for (const sel of facetSelects) {
      const key = sel.getAttribute('data-facet-key');
      const val = sel.value;
      if (key && val) out[key] = val;
    }
    return out;
  }

  function parseRowFacets(li) {
    // "k1:v1,k2:v2" → { k1:v1, k2:v2 }
    const raw = li.getAttribute('data-facets') || '';
    if (!raw) return {};
    const out = {};
    for (const part of raw.split(',')) {
      const idx = part.indexOf(':');
      if (idx <= 0) continue;
      out[part.slice(0, idx)] = part.slice(idx + 1);
    }
    return out;
  }

  function applyFilter() {
    const value = q && !q.disabled ? q.value : '';
    const raw = (value || '').trim().toLowerCase();
    const parts = raw.split(/\s+/).filter(Boolean);
    const tagTerms = parts.filter(p => p.startsWith('#')).map(p => p.slice(1));
    const textTerms = parts.filter(p => !p.startsWith('#'));
    const facetFilters = readFacetFilters();
    const facetKeys = Object.keys(facetFilters);

    items.forEach(li => {
      const tags = (li.getAttribute('data-tags') || '').toLowerCase();
      const title = (li.querySelector('.form-link-title')?.textContent || '').toLowerCase();
      const expr = (li.querySelector('.expr-wrapper')?.textContent || '').toLowerCase();
      const rowFacets = parseRowFacets(li);

      const tagOk = tagTerms.every(t => tags.includes(t));
      const textOk = textTerms.every(t => title.includes(t) || expr.includes(t));
      const facetOk = facetKeys.every(k => rowFacets[k] === facetFilters[k]);

      const show = tagOk && textOk && facetOk;
      li.classList.toggle('hidden', !show);
    });
  }

  if (q && hasTags) {
    q.addEventListener('input', applyFilter);
  }
  for (const sel of facetSelects) {
    paintSelect(sel);
    sel.addEventListener('change', () => {
      paintSelect(sel);
      applyFilter();
    });
  }
  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      for (const sel of facetSelects) {
        sel.value = '';
        paintSelect(sel);
      }
      if (q && !q.disabled) q.value = '';
      applyFilter();
    });
  }

  // Initial paint — covers the "every selected facet" case for back-button
  // returns where the browser restored <select> values from history.
  applyFilter();
})();
