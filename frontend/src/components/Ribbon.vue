<script setup lang="ts">
import { WORKSPACES, type WorkspaceId } from "../workspaces";

defineProps<{ active: WorkspaceId }>();
const emit = defineEmits<{ (e: "select", id: WorkspaceId): void }>();
</script>

<template>
  <nav class="ribbon" aria-label="Workspaces">
    <button
      v-for="w in WORKSPACES"
      :key="w.id"
      class="ribbon-item"
      :class="{ active: w.id === active }"
      :title="w.label"
      :aria-label="w.label"
      :aria-current="w.id === active ? 'page' : undefined"
      @click="emit('select', w.id)"
    >
      <span class="ribbon-icon" aria-hidden="true">{{ w.icon }}</span>
    </button>
  </nav>
</template>
