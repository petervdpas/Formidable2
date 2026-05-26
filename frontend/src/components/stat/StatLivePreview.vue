<script setup lang="ts">
import { ref, watch } from "vue";
import StatGrid from "./StatGrid.vue";
import { type Grid } from "./grid";
import type { Facet } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import { Service as StatSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/stat";

// Live chart preview for a statistical DSL: evaluates the (debounced) DSL
// against the template and renders the result with StatGrid, so a builder
// shows what it builds instead of a raw string. Self-contained - give it a
// template + dsl and it owns the evaluate/render; an eval error falls back to
// emptyText. Reusable by any DSL-authoring surface.
const props = withDefaults(
  defineProps<{
    template: string;
    dsl: string;
    facets?: Facet[];
    label?: string;
    emptyText?: string;
    debounceMs?: number;
  }>(),
  { debounceMs: 250 },
);

const grid = ref<Grid | null>(null);
let timer: ReturnType<typeof setTimeout> | null = null;

async function evaluate() {
  if (!props.dsl || !props.template) {
    grid.value = null;
    return;
  }
  try {
    grid.value = (await StatSvc.EvaluateDSL(props.template, props.dsl)) as unknown as Grid;
  } catch {
    grid.value = null; // the caller surfaces the compile/eval error via emptyText
  }
}

watch(
  () => [props.dsl, props.template] as const,
  () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => void evaluate(), props.debounceMs);
  },
  { immediate: true },
);
</script>

<template>
  <div class="stat-builder-livepreview">
    <span v-if="label" class="stat-builder-field-label">{{ label }}</span>
    <div v-if="grid" class="stat-builder-livepreview-canvas">
      <StatGrid :grid="grid" :facets="facets" />
    </div>
    <p v-else class="muted small">{{ emptyText }}</p>
  </div>
</template>
