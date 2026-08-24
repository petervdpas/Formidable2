// The HTML preview shows a rendered document, not app chrome, so it zooms on
// its own: the reading size of a report has nothing to do with how big the
// surrounding interface should be. The level is a profile setting, like the
// interface font, and the offered levels come from Go.

import { computed, ref } from "vue";
import { useConfig } from "./useConfig";
import { Service as ConfigSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/config";

const levels = ref<number[]>([]);
let loading: Promise<void> | null = null;

function loadLevels(): Promise<void> {
  if (!loading) {
    loading = ConfigSvc.HTMLPreviewZooms()
      .then((v) => {
        levels.value = v ?? [];
      })
      .catch(() => {
        levels.value = [];
      });
  }
  return loading;
}

export function useHTMLPreviewZoom() {
  const { config, update } = useConfig();
  void loadLevels();

  const zoom = computed(() => config.value?.html_preview_zoom || 100);

  return {
    zoom,
    levels: computed(() => levels.value),
    // CSS zoom takes a factor; it reflows the document rather than scaling a
    // bitmap, so text stays crisp and long lines still wrap to the panel.
    factor: computed(() => zoom.value / 100),
    setZoom: (pct: number) => update({ html_preview_zoom: pct }),
  };
}
