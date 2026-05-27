<script setup lang="ts">
// A bounded, vertically-scrolling container for a list that can grow
// past the space a dialog should occupy (e.g. a weights list whose row
// count follows a facet's 16 options, or an unbounded radio/dropdown
// field). The content scrolls inside while the surrounding form controls
// stay in view. maxHeight is a CSS length; it is the only thing that
// varies per use, so it is a bound style rather than static CSS.
withDefaults(defineProps<{ maxHeight?: string }>(), {
  maxHeight: "34vh",
});
</script>

<template>
  <div class="scroll-list" :style="{ maxHeight }">
    <slot />
  </div>
</template>
