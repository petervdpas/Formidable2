<script setup lang="ts">
import { computed } from "vue";
import { useGlobalPluginRun } from "../../composables/useGlobalPluginRun";
import type { Widget } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/formwidget";

// StatusMessageWidget renders a single-line status label driven by
// formidable.run.status(text). All statusmessage widgets in a form
// share the same global `status` ref — the plugin pushes the
// current item's path/name once and every widget re-renders.
defineProps<{ widget: Widget }>();

const { status, running } = useGlobalPluginRun();

const visible = computed<boolean>(() => running.value || status.value !== "");
</script>

<template>
  <div v-if="visible" class="form-widget form-widget-statusmessage">
    <label v-if="widget.label" class="form-widget-label">
      {{ widget.label }}
    </label>
    <p class="form-widget-statusmessage-text">
      {{ status || "—" }}
    </p>
  </div>
</template>
