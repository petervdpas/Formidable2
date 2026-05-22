// scrollToActiveRow - centers the row matching `filename` inside the
// given scroll container, or no-ops when the row is already fully
// visible (so a short list doesn't get a jarring jump). Returns true
// when the row was found, regardless of whether a scroll was needed -
// callers use the return value to gate one-shot "scrolled-for-this-
// context" flags.
//
// Why manual scrollTop math (not scrollIntoView({block:"center"})):
// the native API defers to layout heuristics that, with the sidebar's
// nested overflow ancestors, sometimes anchor the row at an edge
// instead of the middle.
//
// Why getBoundingClientRect, not offsetTop: `offsetTop` is measured
// against the row's `offsetParent` - the nearest *positioned*
// ancestor, which in this layout is the sidebar <aside>, not the
// scroll container. That includes header chrome (title, filters) in
// the offset and overshoots the scroll target. The rect-delta math
// here measures the row's top against the container's own viewport,
// then adds the current scrollTop to get the scroll-content offset
// regardless of intervening layout.
//
// The row is located by its `data-filename` attribute - the workspace
// list items (StorageListItem, TemplateListItem) both stamp this on
// their root `<li>`.

export function scrollToActiveRow(
  container: HTMLElement,
  filename: string,
): boolean {
  if (!filename) return false;
  const rows = container.querySelectorAll<HTMLElement>("[data-filename]");
  for (const row of rows) {
    if (row.dataset.filename !== filename) continue;

    const containerRect = container.getBoundingClientRect();
    const rowRect = row.getBoundingClientRect();
    const rowTopInContent =
      rowRect.top - containerRect.top + container.scrollTop;
    const rowHeight = rowRect.height;

    const viewTop = container.scrollTop;
    const viewBottom = viewTop + container.clientHeight;
    if (rowTopInContent >= viewTop && rowTopInContent + rowHeight <= viewBottom) {
      // Already fully visible - short list or user-scrolled there.
      return true;
    }

    const target = rowTopInContent - (container.clientHeight - rowHeight) / 2;
    const maxScroll = container.scrollHeight - container.clientHeight;
    container.scrollTop = Math.max(0, Math.min(target, maxScroll));
    return true;
  }
  return false;
}
