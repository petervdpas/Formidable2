<script setup lang="ts">
import { useI18n } from "vue-i18n";

defineProps<{ status?: string; profile?: string }>();

const { t } = useI18n();
</script>

<template>
  <footer class="footer">
    <span class="status">{{ status ?? t("status.ready") }}</span>
    <span class="footer-spacer" />
    <span class="profile muted">
      {{ t("footer.user_profile_label") }}
      {{ profile ?? t("footer.user_profile_unknown") }}
    </span>
  </footer>
</template>
