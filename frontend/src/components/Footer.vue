<script setup lang="ts">
defineProps<{ status?: string; profile?: string }>();
</script>

<template>
  <footer class="footer">
    <span class="status">{{ status ?? "Ready." }}</span>
    <span class="footer-spacer" />
    <span class="profile muted">{{ profile ?? "User Profile: —" }}</span>
  </footer>
</template>
