<script setup lang="ts">
import { useI18n } from "vue-i18n";

const { t } = useI18n();
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.info') }}</p>
  <p class="muted">{{ t('workspace.collaboration.gigot.placeholder') }}</p>
</template>
