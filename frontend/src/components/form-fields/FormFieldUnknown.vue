<script setup lang="ts">
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Placeholder for field types that don't have a Vue renderer yet
// (table, image, link, code, latex, api). Shows the type so the user
// knows what's missing and the form still round-trips the value.

defineProps<{
  field: Field;
  modelValue: unknown;
}>();
</script>

<template>
  <div class="unknown-field">
    <span class="unknown-pill">{{ (field.type || "").toUpperCase() }}</span>
    <span class="unknown-note">Renderer pending — value preserved.</span>
  </div>
</template>

<style scoped>
.unknown-field {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 8px var(--space-2);
    border: 1px dashed var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-surface);
    font-size: var(--font-size-sm);
    color: var(--color-muted, #6b7280);
}
.unknown-pill {
    font-family: var(--font-mono);
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--color-surface-2);
    color: var(--color-text);
}
</style>
