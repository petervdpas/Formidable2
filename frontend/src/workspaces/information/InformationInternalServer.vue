<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useWikiServer } from "../../composables/useWikiServer";
import { useToast } from "../../composables/useToast";

const { t } = useI18n();
const toast = useToast();
const {
  status,
  start,
  stop,
  openBrowser,
  openInternal,
  openAPIDocsInBrowser,
  openAPIDocsInWindow,
} = useWikiServer();

const running = computed(() => status.value?.running === true);
const port = computed(() => status.value?.port ?? 0);

const statusText = computed(() =>
  running.value
    ? t("workspace.information.server.status_running", [String(port.value)])
    : t("workspace.information.server.status_stopped"),
);

async function doStart() {
  const r = await start();
  if (r.ok) {
    toast.success("workspace.information.server.toast_started");
  } else {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}

async function doStop() {
  const r = await stop();
  if (r.ok) {
    toast.success("workspace.information.server.toast_stopped");
  } else {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}

async function doOpenBrowser() {
  const r = await openBrowser();
  if (!r.ok) {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}

async function doOpenInternal() {
  const r = await openInternal();
  if (!r.ok) {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}

async function doOpenAPIDocsBrowser() {
  const r = await openAPIDocsInBrowser();
  if (!r.ok) {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}

async function doOpenAPIDocsWindow() {
  const r = await openAPIDocsInWindow();
  if (!r.ok) {
    toast.error("workspace.information.server.toast_action_failed", [r.message]);
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.information.server.info') }}</p>

  <div class="server-status-row">
    <span
      class="server-status-pill"
      :class="running ? 'running' : 'stopped'"
    >{{ statusText }}</span>
  </div>

  <div class="server-action-row">
    <button
      class="tool-btn primary"
      :disabled="running"
      @click="doStart"
    >
      {{ t('workspace.information.server.start') }}
    </button>
    <button
      class="tool-btn"
      :disabled="!running"
      @click="doStop"
    >
      {{ t('workspace.information.server.stop') }}
    </button>
    <span class="server-action-spacer"></span>
    <button
      class="tool-btn"
      :disabled="!running"
      @click="doOpenBrowser"
    >
      {{ t('workspace.information.server.open_in_browser') }}
    </button>
    <button
      class="tool-btn"
      :disabled="!running"
      @click="doOpenInternal"
    >
      {{ t('workspace.information.server.open_internal_wiki') }}
    </button>
  </div>

  <div class="server-action-row">
    <span class="action-row-label">{{ t('workspace.information.server.api_docs_label') }}</span>
    <span class="server-action-spacer"></span>
    <button
      class="tool-btn"
      :disabled="!running"
      @click="doOpenAPIDocsBrowser"
    >
      {{ t('workspace.information.server.open_api_docs_browser') }}
    </button>
    <button
      class="tool-btn"
      :disabled="!running"
      @click="doOpenAPIDocsWindow"
    >
      {{ t('workspace.information.server.open_api_docs_window') }}
    </button>
  </div>

  <p class="muted small">
    {{ t('workspace.information.server.config_hint') }}
  </p>
</template>

<style scoped>
.server-status-row {
  margin: 1em 0;
}
.server-status-pill {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 0.9em;
}
.server-status-pill.running {
  background: var(--color-ok-bg, #d4edda);
  color: var(--color-ok-text, #155724);
  border: 1px solid var(--color-ok-border, #c3e6cb);
}
.server-status-pill.stopped {
  background: var(--color-muted-bg, #e9ecef);
  color: var(--color-muted-text, #495057);
  border: 1px solid var(--color-muted-border, #ced4da);
}
.server-action-row {
  display: flex;
  align-items: center;
  gap: 0.5em;
  flex-wrap: wrap;
  margin: 1em 0;
}
.server-action-spacer {
  flex: 1;
}
.action-row-label {
  font-size: 0.85em;
  font-weight: 600;
  color: var(--color-muted-text, #495057);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
</style>
