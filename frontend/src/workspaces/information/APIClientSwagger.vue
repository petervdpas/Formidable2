<script setup lang="ts">
import { useI18n } from "vue-i18n";

const { t } = useI18n();

defineProps<{
  /** Loopback page serving this client's document, empty until it is ready. */
  url: string;
  loading: boolean;
}>();
</script>

<template>
  <p v-if="loading" class="muted small">{{ t('apiclients.document.loading') }}</p>
  <p v-else-if="!url" class="muted small">{{ t('apiclients.document.no_renderer') }}</p>
  <!-- A frame rather than the panel itself: swagger-ui ships a full reset
       stylesheet that would repaint everything around it. It is served over
       loopback HTTP, so it is also cross-origin from the app, and a document
       authored elsewhere cannot reach in. -->
  <iframe
    v-else
    class="apiclients-swagger"
    :src="url"
    :title="t('apiclients.tab.document')"
  ></iframe>
</template>
