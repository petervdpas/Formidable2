<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import Tabs, { type TabItem } from "../../components/Tabs.vue";
import { Service as LoggingSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/logging";
import { Entry } from "../../../bindings/github.com/petervdpas/formidable2/internal/log/models";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// Information → Logging. Two horizontal tabs: live tail (in-memory
// ring fed by the slog Broadcaster + log:entry events) and the raw
// formidable.log dump. Gating to dev+logging is done by the parent
// (information/index.ts) so this component only worries about render.

const { t } = useI18n();
const toast = useToast();

const tab = ref<"live" | "raw">("live");
const tabItems = computed<TabItem[]>(() => [
  { id: "live", label: t("workspace.information.logging.tab.live") },
  { id: "raw",  label: t("workspace.information.logging.tab.raw") },
]);

const entries = ref<Entry[]>([]);
const rawText = ref<string>("");
const logPath = ref<string>("");

let unsubLog: (() => void) | null = null;

async function loadRecent() {
  try {
    const list = await LoggingSvc.Recent(0);
    entries.value = (list ?? []) as Entry[];
  } catch (err) {
    toast.error("workspace.information.logging.empty", [backendErrMessage(err)]);
  }
}

async function loadFile() {
  try {
    rawText.value = await LoggingSvc.ReadFile();
  } catch (err) {
    toast.error("workspace.information.logging.empty_file", [backendErrMessage(err)]);
  }
}

async function loadPath() {
  try {
    logPath.value = await LoggingSvc.LogPath();
  } catch {
    logPath.value = "";
  }
}

function clearBuffer() {
  entries.value = [];
}

function refreshFile() {
  void loadFile();
}

function fmtTime(t: Date | string | null | undefined): string {
  if (!t) return "";
  const d = typeof t === "string" ? new Date(t) : t;
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString();
}

function fmtAttrs(attrs: Record<string, unknown> | undefined): string {
  if (!attrs) return "";
  return Object.entries(attrs)
    .map(([k, v]) => `${k}=${typeof v === "string" ? v : JSON.stringify(v)}`)
    .join(" ");
}

onMounted(async () => {
  await Promise.all([loadRecent(), loadFile(), loadPath()]);
  unsubLog = Events.On("log:entry", (ev: { data?: Entry } | Entry) => {
    // Wails wraps custom-event payloads in { data, name, ... }; older
    // pathways send the bare object. Handle both.
    const payload = (ev as { data?: Entry })?.data ?? (ev as Entry);
    if (!payload) return;
    entries.value = [...entries.value, payload];
  });
});

onUnmounted(() => {
  if (unsubLog) unsubLog();
  unsubLog = null;
});
</script>

<template>
  <p class="section-info">{{ t('workspace.information.logging.info') }}</p>

  <p v-if="logPath" class="logging-path-row">
    <span class="logging-path-label">{{ t('workspace.information.logging.path_label') }}</span>
    <code class="logging-path">{{ logPath }}</code>
  </p>

  <Tabs v-model="tab" :items="tabItems" orientation="horizontal">
    <template #live>
      <div class="logging-toolbar">
        <button type="button" class="tool-btn" @click="clearBuffer">
          {{ t('workspace.information.logging.clear') }}
        </button>
      </div>
      <ul v-if="entries.length" class="logging-list">
        <li
          v-for="(e, i) in entries"
          :key="i"
          :class="['logging-row', `logging-level--${(e.level || '').toLowerCase()}`]"
        >
          <span class="logging-time">{{ fmtTime(e.time) }}</span>
          <span class="logging-level">{{ e.level }}</span>
          <span class="logging-msg">{{ e.msg }}</span>
          <span v-if="e.attrs" class="logging-attrs">{{ fmtAttrs(e.attrs) }}</span>
        </li>
      </ul>
      <p v-else class="logging-empty">{{ t('workspace.information.logging.empty') }}</p>
    </template>

    <template #raw>
      <div class="logging-toolbar">
        <button type="button" class="tool-btn" @click="refreshFile">
          {{ t('workspace.information.logging.refresh') }}
        </button>
      </div>
      <pre v-if="rawText" class="logging-raw">{{ rawText }}</pre>
      <p v-else class="logging-empty">{{ t('workspace.information.logging.empty_file') }}</p>
    </template>
  </Tabs>
</template>
