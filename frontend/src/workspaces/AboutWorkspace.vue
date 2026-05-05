<script setup lang="ts">
import { ref, onMounted } from "vue";
import SplitPane from "../components/SplitPane.vue";
import { Service as System } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const menus = ["Info", "Links", "License"];
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
  <Teleport defer to="#topbar-content">
    <nav class="topmenu" aria-label="About menu">
      <button v-for="m in menus" :key="m" class="topmenu-item" type="button">
        {{ m }}
      </button>
    </nav>
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane>
    <template #sidebar>
      <h2 class="sidebar-title">About</h2>
      <p class="muted small">Build info and links.</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">Formidable2</h1>
      <p>Wails 3 rewrite of Formidable.</p>
      <dl class="kv">
        <dt>App root</dt>
        <dd v-if="error" class="error">Boot failed: {{ error }}</dd>
        <dd v-else-if="appRoot"><code>{{ appRoot }}</code></dd>
        <dd v-else class="muted">Loading…</dd>
      </dl>
    </template>
  </SplitPane>
</template>
