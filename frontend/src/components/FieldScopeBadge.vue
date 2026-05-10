<script setup lang="ts">
import { useConfig } from "../composables/useConfig";

defineProps<{ level: number | null | undefined }>();

const { config } = useConfig();
</script>

<template>
  <span v-if="config?.development_enable" class="badge badge-level">L{{ level ?? 0 }}</span>
</template>
