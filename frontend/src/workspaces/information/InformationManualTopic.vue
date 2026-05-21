<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Service as ManualSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/manual";
import { Service as RenderSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/render";

const props = defineProps<{ topic: string }>();
const { t, locale } = useI18n();

const html = ref("");
const loading = ref(true);
const error = ref(false);

async function load(): Promise<void> {
  loading.value = true;
  error.value = false;
  try {
    const md = await ManualSvc.GetTopic(props.topic, locale.value);
    html.value = await RenderSvc.RenderHTML(md);
  } catch {
    html.value = "";
    error.value = true;
  } finally {
    loading.value = false;
  }
}

watch(() => [props.topic, locale.value], () => void load(), { immediate: true });
</script>

<template>
  <p v-if="loading" class="muted small">{{ t('workspace.information.manual.loading') }}</p>
  <p v-else-if="error" class="muted small">{{ t('workspace.information.manual.error') }}</p>
  <div v-else class="manual-body markdown-body" v-html="html"></div>
</template>
