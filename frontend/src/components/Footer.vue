<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useConfig } from "../composables/useConfig";
import { useStatusBar } from "../composables/useStatusBar";

defineProps<{ profile?: string }>();

const { t } = useI18n();
const { config, profileFilename } = useConfig();
const { text: statusText, variant: statusVariant } = useStatusBar();

// Mirrors the fallback the Go side uses in ListAvailableProfiles:
// profile_name → author_name → filename → em-dash placeholder.
const activeProfileLabel = computed(() => {
  const name = config.value?.profile_name?.trim();
  if (name) return name;
  const author = config.value?.author_name?.trim();
  if (author) return author;
  if (profileFilename.value) return profileFilename.value;
  return t("footer.user_profile_unknown");
});
</script>

<template>
  <footer class="footer">
    <span class="status" :class="`status-${statusVariant}`">
      {{ statusText || t("status.ready") }}
    </span>
    <span class="footer-spacer" />
    <span class="profile muted">
      {{ t("footer.user_profile_label") }}
      {{ profile ?? activeProfileLabel }}
    </span>
  </footer>
</template>
