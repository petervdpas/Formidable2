<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useConfig } from "../../composables/useConfig";
import GitConnection from "../../components/collaboration/GitConnection.vue";
import GigotConnection from "../../components/collaboration/GigotConnection.vue";

// Single Collaboration main view. Branches on config.remote_backend
// to pick the section-info text + connection form. The "none" case
// is unreachable from here in practice - ribbon ghosting + App.vue
// redirect short-circuits it - but the v-if guards keep this view
// safe to render unconditionally.
const { t } = useI18n();
const { config } = useConfig();

const backend = computed(() => config.value?.remote_backend ?? "none");
</script>

<template>
  <template v-if="backend === 'git'">
    <p class="section-info">{{ t('workspace.collaboration.git.info') }}</p>
    <GitConnection />
  </template>
  <template v-else-if="backend === 'gigot'">
    <p class="section-info">{{ t('workspace.collaboration.gigot.info') }}</p>
    <GigotConnection />
  </template>
</template>
