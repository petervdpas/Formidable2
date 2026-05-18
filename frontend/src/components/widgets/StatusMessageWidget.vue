<script setup lang="ts">
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";

// StatusMessageWidget renders a single-line status label driven by
// formidable.run.status(text). All statusmessage widgets in a form
// share the same global `status` ref — the plugin pushes the
// current item's path/name once and every widget re-renders. Always
// rendered so the author's form layout is the same before / during /
// after a run; empty text shows an em-dash placeholder at idle.
defineProps<{ widget: Widget }>();

const { status } = useGlobalPluginRun();
</script>

<template>
  <div class="form-widget form-widget-statusmessage">
    <label v-if="widget.label" class="form-widget-label">
      {{ widget.label }}
    </label>
    <p class="form-widget-statusmessage-text">
      {{ status || "—" }}
    </p>
  </div>
</template>
