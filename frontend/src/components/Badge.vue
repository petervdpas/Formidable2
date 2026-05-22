<script setup lang="ts">
// Badge wraps the existing .badge / .badge-ok / .badge-warn /
// .badge-accent CSS primitives in styles/badge.css so callers don't
// have to keep stringing class arrays together. Slot in the content,
// pick a variant - that's it.
//
// Variants:
//   - "ok"     - green outline, positive state ("saved", "connected")
//   - "warn"   - error-tinted outline, missing-required state
//   - "accent" - accent fill, primary-call-out
//   - omit     - neutral chrome, plain informational pill

defineProps<{
  variant?: "ok" | "warn" | "accent";
}>();
</script>

<template>
  <span :class="['badge', variant && `badge-${variant}`]">
    <slot />
  </span>
</template>
