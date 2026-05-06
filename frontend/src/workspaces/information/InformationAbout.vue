<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { Service as System } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const { t } = useI18n();
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
  <p>{{ t('workspace.information.subtitle') }}</p>
  <dl class="kv">
    <dt>{{ t('workspace.information.app_root') }}</dt>
    <dd v-if="error" class="error">{{ t('workspace.information.boot_failed', [error]) }}</dd>
    <dd v-else-if="appRoot"><code>{{ appRoot }}</code></dd>
    <dd v-else class="muted">{{ t('workspace.information.loading') }}</dd>
  </dl>
</template>
