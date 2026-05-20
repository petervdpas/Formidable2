<script setup lang="ts">
import { computed } from "vue";
import { useFacetMeta } from "../composables/useFacetMeta";

// FacetIcon renders one of the 16 facet glyphs as inline SVG using
// the catalog the backend ships via TemplateSvc.FacetMeta. Falls back
// to fa-flag when the requested icon isn't in the palette so a stale
// template reference still gets a visible glyph instead of a void.
//
// Same SVGs power the embedded wiki — single source of truth in
// internal/modules/template/icons/, parsed once per process and
// projected through Wails to every facet UI on the frontend.

const props = defineProps<{ icon?: string | null }>();
const { iconSvgs } = useFacetMeta();

const spec = computed(() => {
  const map = iconSvgs.value;
  const want = props.icon ?? "";
  return map[want] ?? map["fa-flag"] ?? null;
});
</script>

<template>
  <svg
    v-if="spec"
    class="facet-icon-svg"
    :viewBox="spec.viewBox"
    aria-hidden="true"
  >
    <path fill="currentColor" :d="spec.path" />
  </svg>
</template>
