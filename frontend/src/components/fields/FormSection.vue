<script setup lang="ts">
defineProps<{ title?: string }>();
</script>

<template>
  <section class="form-section">
    <h2 v-if="title" class="form-section-title">{{ title }}</h2>
    <slot />
  </section>
</template>
