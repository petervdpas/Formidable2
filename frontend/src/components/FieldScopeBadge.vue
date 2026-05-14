<script setup lang="ts">
import Badge from "./Badge.vue";
import { useConfig } from "../composables/useConfig";

defineProps<{ level: number | null | undefined }>();

const { config } = useConfig();
</script>

<template>
  <Badge v-if="config?.development_enable" class="badge-level">
    L{{ level ?? 0 }}
  </Badge>
</template>
