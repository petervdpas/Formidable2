<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { Field } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/template";

// Placeholder for field types that don't have a Vue renderer yet
// (image, link, code, latex, api). Shows the type so the user
// knows what's missing and the form still round-trips the value.

defineProps<{
  field: Field;
  modelValue: unknown;
}>();

const { t } = useI18n();
</script>

<template>
  <div class="unknown-field">
    <span class="unknown-pill">{{ (field.type || "").toUpperCase() }}</span>
    <span class="unknown-note">{{ t('workspace.storage.field.renderer_pending') }}</span>
  </div>
</template>

