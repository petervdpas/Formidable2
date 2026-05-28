import { nextTick, onScopeDispose, ref, watch, type Ref } from "vue";
import { scrollToActiveRow } from "../utils/scrollToActiveRow";
import { useConfig } from "./useConfig";
import { useStatusBar } from "./useStatusBar";

// useTemplateSelection
// Two-way binding between the templates sidebar selection and
// config.selected_template, plus the one-shot "scroll the active row
// into view" behavior that has to wait for layout to settle on first
// paint (the splash blocks flex layout, so .sidebar-scroll reads as
// ~8px until the splash clears).
//
// The composable takes refs from the templates store rather than
// owning them, so writes propagate back into the singleton and stay
// observable by everything else (storage workspace's picker, etc.).
// It returns the scroll-container ref the template binds with
// ref="listScrollEl".
//
// Why the singletons get called here: useConfig() and useStatusBar()
// are module-scoped, so any caller gets the same instance. No reason
// to pass them in.
export function useTemplateSelection(
  filenames: Ref<string[]>,
  selectedFilename: Ref<string>,
) {
  const { config, update: updateConfig } = useConfig();
  const statusBar = useStatusBar();

  // ── Direction 1: config.selected_template → sidebar highlight ──────
  // Fires whenever the config value or the loaded filenames list
  // changes (need both - config can name a template that hasn't loaded
  // yet during early boot).
  watch(
    [() => config.value?.selected_template ?? "", filenames],
    ([want, list]) => {
      if (!list.length || !want) return;
      if (!list.includes(want)) return;
      if (selectedFilename.value !== want) selectedFilename.value = want;
    },
    { immediate: true },
  );

  // ── Direction 2: sidebar click → config ────────────────────────────
  // Skip when config already reflects this choice (avoids a redundant
  // write triggered by the mirror watcher above).
  watch(selectedFilename, (fn) => {
    if (!fn) return;
    if (config.value?.selected_template !== fn) {
      void updateConfig({ selected_template: fn });
    }
    statusBar.setSelected(fn);
  });

  // ── First-paint scroll-into-view ───────────────────────────────────
  // Two cases mirror the storage workspace:
  //   - In-app navigation: layout is settled, the scroll runs
  //     synchronously on the first attempt.
  //   - First startup: the splash blocks flex layout, so
  //     .sidebar-scroll reads as ~8px until layout settles. The
  //     ResizeObserver below retries on each resize until the
  //     container has a real viewport.
  // hasScrolled stays true after the first successful scroll so save /
  // refresh / clicks don't yank the viewport.
  const listScrollEl = ref<HTMLElement | null>(null);
  let hasScrolled = false;
  let pendingScrollObserver: ResizeObserver | null = null;

  function cancelPendingScroll() {
    pendingScrollObserver?.disconnect();
    pendingScrollObserver = null;
  }

  watch(
    [filenames, selectedFilename],
    async ([list, want]) => {
      if (hasScrolled) return;
      if (!list.length || !want) return;
      if (!list.includes(want)) return;
      cancelPendingScroll();
      await nextTick();
      const container = listScrollEl.value;
      if (!container) return;

      const SETTLED_MIN_HEIGHT = 140;
      const attempt = (): boolean => {
        if (container.clientHeight < SETTLED_MIN_HEIGHT) return false;
        if (scrollToActiveRow(container, want)) {
          hasScrolled = true;
          return true;
        }
        return false;
      };

      if (attempt()) return;

      const ro = new ResizeObserver(() => {
        if (attempt()) {
          ro.disconnect();
          if (pendingScrollObserver === ro) pendingScrollObserver = null;
        }
      });
      ro.observe(container);
      pendingScrollObserver = ro;
    },
    { immediate: true },
  );

  onScopeDispose(cancelPendingScroll);

  return { listScrollEl };
}
