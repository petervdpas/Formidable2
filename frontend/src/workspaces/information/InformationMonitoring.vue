<script setup lang="ts">
import { useI18n } from "vue-i18n";

const { t } = useI18n();
</script>

<template>
  <p class="section-info">{{ t('workspace.information.monitoring.info') }}</p>
  <p class="muted">{{ t('workspace.information.monitoring.placeholder') }}</p>
</template>
