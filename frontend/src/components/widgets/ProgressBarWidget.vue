<script setup lang="ts">
import { computed } from "vue";
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";

// ProgressBarWidget renders a single bar driven by formidable.run.bar.
// All progressbar widgets in a form read the same global `bar` ref —
// a plugin can call formidable.run.bar(done, total) once per item
// and every bar in the form re-renders. Total == 0 means
// indeterminate; the CSS class drives the sliding animation. Always
// rendered so the author's form layout stays consistent before / after
// a run — empty bar at idle, fills during.
defineProps<{ widget: Widget }>();

const { bar, running } = useGlobalPluginRun();

const pct = computed<number>(() => {
  const b = bar.value;
  if (!b || b.total <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((b.done / b.total) * 100)));
});

const indeterminate = computed<boolean>(() => {
  // Indeterminate only WHILE running and the plugin hasn't sent a
  // total yet; an idle bar shows a flat 0% fill, not an animated
  // slider, so the form looks calm before Run is clicked.
  const b = bar.value;
  return running.value && (!b || b.total <= 0);
});
</script>

<template>
  <div class="form-widget form-widget-progressbar">
    <label v-if="widget.label" class="form-widget-label">
      {{ widget.label }}
    </label>
    <div
      class="form-widget-progressbar-bar"
      :class="{ 'is-indeterminate': indeterminate }"
    >
      <div
        class="form-widget-progressbar-fill"
        :style="!indeterminate ? { width: pct + '%' } : undefined"
      ></div>
    </div>
    <p v-if="bar && bar.total > 0" class="form-widget-progressbar-count muted small">
      {{ bar.done }} / {{ bar.total }}
    </p>
  </div>
</template>
