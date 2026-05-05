<script setup lang="ts">
import { ref, onMounted } from "vue";
import { Service as System } from "../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const appRoot = ref<string>("");
const error = ref<string>("");

onMounted(async () => {
  try {
    appRoot.value = await System.GetAppRoot();
  } catch (err) {
    error.value = String(err);
  }
});
</script>

<template>
  <main>
    <p v-if="error">Boot failed: {{ error }}</p>
    <p v-else-if="appRoot">Formidable2 — appRoot: {{ appRoot }}</p>
    <p v-else>Loading…</p>
  </main>
</template>
