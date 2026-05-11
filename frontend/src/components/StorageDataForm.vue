<script setup lang="ts">
/*
 * StorageDataForm — presentational wrapper around the form-fields
 * renderer for a storage entry. Sits below the sticky StorageMetaBlock
 * and renders the active template's fields in template order.
 */
import FormLoopFields from "./form-fields/FormLoopFields.vue";
import type { Template } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/template";
import type { LoopGroup } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/form";

defineProps<{
  template: Template;
  values: Record<string, unknown>;
  loopGroups: LoopGroup[];
}>();
</script>

<template>
  <div class="form-fields">
    <FormLoopFields
      :fields="template.fields"
      :start-offset="0"
      :values="values"
      :loop-groups="loopGroups"
    />
  </div>
</template>
