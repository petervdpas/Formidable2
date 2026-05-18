<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { CoverDescriptor } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf/models";

defineProps<{
  cover: CoverDescriptor;
  active: boolean;
  isSeed: boolean;
}>();

const emit = defineEmits<{
  (e: "pick", name: string): void;
  (e: "delete", name: string): void;
  (e: "export", name: string): void;
}>();

const { t } = useI18n();
</script>

<template>
  <li :class="['pdf-covers-row', { active, invalid: !cover.ok }]">
    <button class="pdf-covers-row-main" type="button" @click="emit('pick', cover.name)">
      <span class="pdf-covers-row-label">
        {{ cover.label || cover.name }}
        <span v-if="isSeed" class="pdf-covers-pill pdf-covers-pill-seed">
          {{ t('pdf.covers.pill.seed') }}
        </span>
        <span v-if="!cover.ok" class="pdf-covers-pill pdf-covers-pill-invalid">
          {{ t('pdf.covers.pill.invalid') }}
        </span>
      </span>
      <span v-if="cover.description" class="pdf-covers-row-desc">{{ cover.description }}</span>
      <code class="pdf-covers-row-name">{{ cover.name }}.html</code>
    </button>

    <button
      class="tool-btn pdf-covers-row-action"
      type="button"
      :title="t('pdf.covers.action.export')"
      @click="emit('export', cover.name)"
    >
      <i class="fa-solid fa-file-export" aria-hidden="true"></i>
    </button>
    <button
      class="tool-btn pdf-covers-row-action"
      type="button"
      :title="t(isSeed ? 'pdf.covers.action.reset' : 'pdf.covers.action.delete')"
      @click="emit('delete', cover.name)"
    >
      <i :class="isSeed ? 'fa-solid fa-arrow-rotate-left' : 'fa-solid fa-trash'" aria-hidden="true"></i>
    </button>
  </li>
</template>
