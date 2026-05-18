<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Service as PdfSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/pdf";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";

const { t, locale } = useI18n();

const html = ref("");
const loading = ref(true);

onMounted(async () => {
  try {
    const md = await PdfSvc.GetDirectivesDoc(locale.value);
    html.value = await RenderSvc.RenderHTML(md);
  } catch {
    html.value = "";
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <p v-if="loading" class="muted small">{{ t('pdf.directives.loading') }}</p>
  <div v-else class="pdf-directives-body markdown-body" v-html="html"></div>
</template>
