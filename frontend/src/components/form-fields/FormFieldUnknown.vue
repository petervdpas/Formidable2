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

