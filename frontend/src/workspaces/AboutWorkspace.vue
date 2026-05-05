<script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import SplitPane from "../components/SplitPane.vue";
import { useRestartGate } from "../composables/useRestartGate";
import { Service as System } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const { t } = useI18n();
const { bootConfig } = useRestartGate();

const sidebarWidth = computed(() => bootConfig.value?.sidebar_width || 280);

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
    <span class="topbar-spacer"></span>
  </Teleport>

  <SplitPane :initial="sidebarWidth">
    <template #sidebar>
      <h2 class="sidebar-title">{{ t('workspace.about.sidebar_title') }}</h2>
      <p class="muted small">{{ t('workspace.about.placeholder_side') }}</p>
    </template>
    <template #main>
      <h1 class="workspace-heading">{{ t('workspace.about.title') }}</h1>
      <p>{{ t('workspace.about.subtitle') }}</p>
      <dl class="kv">
        <dt>{{ t('workspace.about.app_root') }}</dt>
        <dd v-if="error" class="error">{{ t('workspace.about.boot_failed', [error]) }}</dd>
        <dd v-else-if="appRoot"><code>{{ appRoot }}</code></dd>
        <dd v-else class="muted">{{ t('workspace.about.loading') }}</dd>
      </dl>
    </template>
  </SplitPane>
</template>
