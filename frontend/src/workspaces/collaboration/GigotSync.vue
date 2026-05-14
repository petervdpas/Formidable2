<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import ProgressBar from "../../components/ProgressBar.vue";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import type {
  LedgerSummary,
  HeadResponse,
  SyncProgress,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { useConfig } from "../../composables/useConfig";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const contextFolder = computed(() => config.value?.context_folder ?? "");
const baseURL = computed(() => config.value?.gigot_base_url ?? "");
const repoName = computed(() => config.value?.gigot_repo_name ?? "");

const configured = computed(
  () =>
    contextFolder.value.trim() !== ""
    && baseURL.value.trim() !== ""
    && repoName.value.trim() !== "",
);

const summary = ref<LedgerSummary | null>(null);
const head = ref<HeadResponse | null>(null);
const headError = ref("");
const loading = ref(false);
const errorMsg = ref("");

const pushing = ref(false);
const pulling = ref(false);
const syncing = ref(false);

const progressCurrent = ref(0);
const progressTotal = ref(0);
const progressPath = ref("");

const inFlight = computed(
  () => pushing.value || pulling.value || syncing.value,
);
const canAct = computed(() => !inFlight.value && configured.value);
const hasLedger = computed(() => (summary.value?.version ?? "") !== "");
const hasPending = computed(() => {
  const s = summary.value;
  if (!s) return false;
  return (s.changed?.length ?? 0) > 0 || (s.deleted?.length ?? 0) > 0;
});

// HEAD probe state: "unknown" before first probe / on error;
// "match" when remote == ledger version; "behind" when remote moved.
const remoteState = computed<"unknown" | "match" | "behind">(() => {
  if (!head.value) return "unknown";
  const remote = head.value.version ?? "";
  const local = summary.value?.version ?? "";
  if (!remote || !local) return "unknown";
  return remote === local ? "match" : "behind";
});

const shortHash = (v: string | undefined | null) =>
  v ? v.slice(0, 8) : "";

const progressLabel = computed(() => {
  if (pulling.value) return t("workspace.collaboration.gigot.sync.pull.running");
  if (pushing.value) return t("workspace.collaboration.gigot.sync.push.running");
  if (syncing.value) return t("workspace.collaboration.gigot.sync.sync.running");
  return "";
});

function resetProgress() {
  progressCurrent.value = 0;
  progressTotal.value = 0;
  progressPath.value = "";
}

let unsubscribeProgress: (() => void) | null = null;

onMounted(() => {
  unsubscribeProgress = Events.On(
    "gigot:sync_progress",
    (ev: { data?: SyncProgress } | SyncProgress) => {
      const data = (ev as { data?: SyncProgress })?.data ?? (ev as SyncProgress);
      if (!data || typeof data !== "object") return;
      progressCurrent.value = data.current ?? 0;
      progressTotal.value = data.total ?? 0;
      progressPath.value = data.path ?? "";
    },
  );
  void load(false);
});

onBeforeUnmount(() => {
  unsubscribeProgress?.();
  unsubscribeProgress = null;
});

watch(
  () => [contextFolder.value, baseURL.value, repoName.value] as const,
  () => void load(false),
);

// Race guard on the refresh path — only the most recent fetch wins.
let reqId = 0;

async function load(announce: boolean) {
  if (!configured.value) {
    summary.value = null;
    head.value = null;
    headError.value = "";
    errorMsg.value = "";
    return;
  }
  const my = ++reqId;
  loading.value = true;
  errorMsg.value = "";
  headError.value = "";
  try {
    const s = await GigotSvc.LedgerSummary();
    if (my !== reqId) return;
    summary.value = s ?? null;
  } catch (err) {
    if (my !== reqId) return;
    errorMsg.value = backendErrMessage(err);
    summary.value = null;
    if (announce) {
      toast.error("workspace.collaboration.gigot.sync.refresh.error", [errorMsg.value]);
    }
  }

  try {
    const h = await GigotSvc.Head();
    if (my !== reqId) return;
    head.value = h ?? null;
  } catch (err) {
    if (my !== reqId) return;
    head.value = null;
    headError.value = backendErrMessage(err);
  } finally {
    if (my === reqId) loading.value = false;
  }
}

async function doPush() {
  if (!canAct.value) return;
  resetProgress();
  pushing.value = true;
  try {
    const res = await GigotSvc.PushLocal();
    if (!res) throw new Error("no response");
    if (res.noop) {
      toast.info("workspace.collaboration.gigot.sync.push.noop");
    } else {
      toast.success("workspace.collaboration.gigot.sync.push.success", [
        String(res.pushed ?? 0),
        String(res.deleted ?? 0),
        res.version ?? "",
      ]);
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.push.error", [backendErrMessage(err)]);
  } finally {
    pushing.value = false;
    resetProgress();
  }
}

async function doPull() {
  if (!canAct.value) return;
  resetProgress();
  pulling.value = true;
  try {
    const res = await GigotSvc.PullLocal();
    if (!res) throw new Error("no response");
    toast.success("workspace.collaboration.gigot.sync.pull.success", [
      String(res.files ?? 0),
      String(res.deleted ?? 0),
      res.version ?? "",
    ]);
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.pull.error", [backendErrMessage(err)]);
  } finally {
    pulling.value = false;
    resetProgress();
  }
}

async function doSync() {
  if (!canAct.value) return;
  resetProgress();
  syncing.value = true;
  try {
    const res = await GigotSvc.Sync();
    if (!res) throw new Error("no response");
    if (res.noop) {
      toast.info("workspace.collaboration.gigot.sync.sync.noop");
    } else {
      toast.success("workspace.collaboration.gigot.sync.sync.success", [res.version ?? ""]);
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.sync.error", [backendErrMessage(err)]);
  } finally {
    syncing.value = false;
    resetProgress();
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.gigot.sync.info') }}</p>

  <div v-if="!configured" class="gigot-sync-note">
    {{ t('workspace.collaboration.gigot.sync.not_configured') }}
  </div>

  <template v-else>
    <div class="gigot-sync-status">
      <div class="gigot-sync-row">
        <span class="gigot-sync-label">{{ t('workspace.collaboration.gigot.sync.last_version') }}:</span>
        <code v-if="hasLedger" class="gigot-sync-hash">{{ shortHash(summary?.version) }}</code>
        <span v-else class="gigot-sync-muted">—</span>
      </div>
      <div class="gigot-sync-row">
        <span class="gigot-sync-label">{{ t('workspace.collaboration.gigot.sync.last_sync') }}:</span>
        <span v-if="summary?.lastSync">{{ summary.lastSync }}</span>
        <span v-else class="gigot-sync-muted">{{ t('workspace.collaboration.gigot.sync.never') }}</span>
      </div>
      <div class="gigot-sync-row">
        <span class="gigot-sync-label">{{ t('workspace.collaboration.gigot.sync.remote_label') }}:</span>
        <code v-if="head?.version" class="gigot-sync-hash">{{ shortHash(head.version) }}</code>
        <span v-else class="gigot-sync-muted">{{ t('workspace.collaboration.gigot.sync.remote_unknown') }}</span>
        <span v-if="remoteState === 'match'" class="gigot-sync-tag tag-clean">{{ t('workspace.collaboration.gigot.sync.remote_match') }}</span>
        <span v-else-if="remoteState === 'behind'" class="gigot-sync-tag tag-behind">{{ t('workspace.collaboration.gigot.sync.remote_behind', [shortHash(head?.version)]) }}</span>
      </div>
      <div v-if="summary" class="gigot-sync-row gigot-sync-muted">
        {{ t('workspace.collaboration.gigot.sync.scanned', [String(summary.scanned ?? 0)]) }}
      </div>
      <div v-if="headError" class="gigot-sync-row gigot-sync-error">
        {{ t('workspace.collaboration.gigot.sync.remote_error', [headError]) }}
      </div>
      <div v-if="errorMsg" class="gigot-sync-row gigot-sync-error">
        {{ t('workspace.collaboration.gigot.sync.refresh.error', [errorMsg]) }}
      </div>
    </div>

    <div v-if="!hasLedger && summary && !errorMsg" class="gigot-sync-note">
      {{ t('workspace.collaboration.gigot.sync.no_ledger') }}
    </div>

    <div v-if="summary && !hasPending && hasLedger" class="gigot-sync-clean">
      {{ t('workspace.collaboration.gigot.sync.clean') }}
    </div>

    <details v-if="summary && (summary.changed?.length ?? 0) > 0" class="gigot-sync-pending">
      <summary>{{ t('workspace.collaboration.gigot.sync.pending_changed', [String(summary.changed?.length ?? 0)]) }}</summary>
      <ul>
        <li v-for="p in summary.changed" :key="`c:${p}`">{{ p }}</li>
      </ul>
    </details>

    <details v-if="summary && (summary.deleted?.length ?? 0) > 0" class="gigot-sync-pending">
      <summary>{{ t('workspace.collaboration.gigot.sync.pending_deleted', [String(summary.deleted?.length ?? 0)]) }}</summary>
      <ul>
        <li v-for="p in summary.deleted" :key="`d:${p}`">{{ p }}</li>
      </ul>
    </details>

    <ProgressBar
      :active="inFlight"
      :label="progressLabel"
      :current="pulling || syncing ? progressCurrent : 0"
      :total="pulling || syncing ? progressTotal : 0"
    />

    <div class="gigot-sync-actions">
      <button
        type="button"
        class="tool-btn"
        :disabled="loading || inFlight || !configured"
        @click="load(true)"
      >
        {{ loading ? t('workspace.collaboration.gigot.sync.refresh.running') : t('workspace.collaboration.gigot.sync.refresh') }}
      </button>
      <button
        type="button"
        class="tool-btn"
        :disabled="!canAct"
        @click="doPush"
      >
        {{ pushing ? t('workspace.collaboration.gigot.sync.push.running') : t('workspace.collaboration.gigot.sync.push.button') }}
      </button>
      <button
        type="button"
        class="tool-btn"
        :disabled="!canAct"
        @click="doPull"
      >
        {{ pulling ? t('workspace.collaboration.gigot.sync.pull.running') : t('workspace.collaboration.gigot.sync.pull.button') }}
      </button>
      <button
        type="button"
        class="tool-btn primary"
        :disabled="!canAct"
        @click="doSync"
      >
        {{ syncing ? t('workspace.collaboration.gigot.sync.sync.running') : t('workspace.collaboration.gigot.sync.sync.button') }}
      </button>
    </div>
  </template>
</template>
