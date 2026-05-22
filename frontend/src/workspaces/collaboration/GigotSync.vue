<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import ProgressBar from "../../components/ProgressBar.vue";
import {
  FormSection,
  FormRow,
  TextareaField,
} from "../../components/fields";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import type {
  LedgerSummary,
  HeadResponse,
  SyncProgress,
  Destination,
  RepoContextResponse,
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
const mirroring = ref(false);
const message = ref("");

const destinations = ref<Destination[]>([]);
const meRole = ref("");
const meAbilities = ref<string[]>([]);

const progressCurrent = ref(0);
const progressTotal = ref(0);
const progressPath = ref("");

const inFlight = computed(() => pushing.value || pulling.value || mirroring.value);
const canAct = computed(() => !inFlight.value && configured.value);
const hasLedger = computed(() => (summary.value?.version ?? "") !== "");
const hasPending = computed(() => {
  const s = summary.value;
  if (!s) return false;
  return (s.changed?.length ?? 0) > 0 || (s.deleted?.length ?? 0) > 0;
});
const messageProvided = computed(() => message.value.trim() !== "");

// HEAD probe state: "unknown" before first probe / on error;
// "match" when remote == ledger version; "behind" when remote moved.
const remoteState = computed<"unknown" | "match" | "behind">(() => {
  if (!head.value) return "unknown";
  const remote = head.value.version ?? "";
  const local = summary.value?.version ?? "";
  if (!remote || !local) return "unknown";
  return remote === local ? "match" : "behind";
});

// Button gates - Push/Pull are the fine-grained operations on this
// panel; the bundled one-click Sync lives on Current Service. The
// goal is to make each button mean exactly one thing, so the user
// doesn't have to guess which is "safe" to click.
//
//   Push: pending changes + commit message
//   Pull: remote moved + no pending changes (avoids clobbering)
//
// Pull is disabled when the remote matches the local ledger AND
// when there are pending local changes - both cases boil down to
// "Pull would either do nothing or destroy work."
const canPush = computed(() => canAct.value && hasPending.value && messageProvided.value);
const canPull = computed(() => canAct.value && !hasPending.value && remoteState.value !== "match");

const hasMirrors = computed(() => destinations.value.length > 0);
const hasMirrorAbility = computed(() => {
  const r = meRole.value;
  if (r !== "admin" && r !== "maintainer") return false;
  return meAbilities.value.includes("mirror");
});
const allMirrorsInSync = computed(
  () => hasMirrors.value && destinations.value.every(d => d.remote_status === "in_sync"),
);
// Mirror push is force-mirror against the server's HEAD. Pending local
// changes haven't been Push'd yet, so the mirror would carry the stale
// pre-Push state - block it until the local commit lands. Also skip
// when every mirror reports in_sync - there is nothing to push.
const canMirror = computed(
  () =>
    canAct.value
    && hasMirrors.value
    && hasMirrorAbility.value
    && !hasPending.value
    && !allMirrorsInSync.value,
);

// Per-button disabled tooltips. Empty string when the button is
// enabled - the v-bind below feeds the attr only when non-empty.
const pushDisabledHint = computed(() => {
  if (!canAct.value || !configured.value) return "";
  if (!hasPending.value) return t("workspace.collaboration.gigot.sync.push.disabled_no_pending");
  if (!messageProvided.value) return t("workspace.collaboration.gigot.sync.message_required");
  return "";
});
const pullDisabledHint = computed(() => {
  if (!canAct.value || !configured.value) return "";
  if (hasPending.value) return t("workspace.collaboration.gigot.sync.pull.disabled_pending");
  if (remoteState.value === "match") return t("workspace.collaboration.gigot.sync.pull.disabled_match");
  return "";
});
const mirrorDisabledHint = computed(() => {
  if (!canAct.value || !configured.value) return "";
  if (hasPending.value) return t("workspace.collaboration.gigot.sync.mirror.disabled_pending");
  if (allMirrorsInSync.value) return t("workspace.collaboration.gigot.sync.mirror.disabled_in_sync");
  return "";
});

const shortHash = (v: string | undefined | null) =>
  v ? v.slice(0, 8) : "";

const progressLabel = computed(() => {
  if (pulling.value) return t("workspace.collaboration.gigot.sync.pull.running");
  if (pushing.value) return t("workspace.collaboration.gigot.sync.push.running");
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

// Race guard on the refresh path - only the most recent fetch wins.
let reqId = 0;

async function load(announce: boolean) {
  if (!configured.value) {
    summary.value = null;
    head.value = null;
    headError.value = "";
    errorMsg.value = "";
    destinations.value = [];
    meRole.value = "";
    meAbilities.value = [];
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
  }

  try {
    const ctx: RepoContextResponse | null = await GigotSvc.Context();
    if (my !== reqId) return;
    meRole.value = ctx?.user?.role ?? "";
    meAbilities.value = ctx?.subscription?.abilities ?? [];
  } catch {
    if (my !== reqId) return;
    meRole.value = "";
    meAbilities.value = [];
  }

  try {
    const ds = await GigotSvc.Destinations();
    if (my !== reqId) return;
    destinations.value = ds ?? [];
  } catch {
    if (my !== reqId) return;
    destinations.value = [];
  } finally {
    if (my === reqId) {
      loading.value = false;
      window.dispatchEvent(new CustomEvent("formidable:gigot-refreshed"));
    }
  }
}

async function doPush() {
  if (!canPush.value) return;
  resetProgress();
  pushing.value = true;
  try {
    const res = await GigotSvc.PushLocal(message.value);
    if (!res) throw new Error("no response");
    if (res.noop) {
      toast.info("workspace.collaboration.gigot.sync.push.noop");
    } else {
      toast.success("workspace.collaboration.gigot.sync.push.success", [
        String(res.pushed ?? 0),
        String(res.deleted ?? 0),
        shortHash(res.version),
      ]);
      message.value = "";
    }
    await load(false);
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.push.error", [backendErrMessage(err)]);
  } finally {
    pushing.value = false;
    resetProgress();
  }
}

async function doMirror() {
  if (!canMirror.value) return;
  mirroring.value = true;
  let ok = 0;
  const failures: string[] = [];
  try {
    for (const d of destinations.value) {
      if (!d.id) continue;
      try {
        await GigotSvc.DestinationSync(d.id);
        ok += 1;
      } catch (err) {
        const label = d.url || d.id;
        failures.push(`${label}: ${backendErrMessage(err)}`);
      }
    }
    if (failures.length === 0) {
      toast.success("workspace.collaboration.gigot.sync.mirror.success", [String(ok)]);
    } else if (ok === 0) {
      toast.error("workspace.collaboration.gigot.sync.mirror.error", [failures.join("; ")]);
    } else {
      toast.error("workspace.collaboration.gigot.sync.mirror.partial", [
        String(ok),
        String(failures.length),
        failures.join("; "),
      ]);
    }
    await load(false);
  } finally {
    mirroring.value = false;
  }
}

async function doPull() {
  if (!canPull.value) return;
  resetProgress();
  pulling.value = true;
  try {
    const res = await GigotSvc.PullLocal();
    if (!res) throw new Error("no response");
    toast.success("workspace.collaboration.gigot.sync.pull.success", [
      String(res.files ?? 0),
      String(res.deleted ?? 0),
      shortHash(res.version),
    ]);
    await load(false);
    window.dispatchEvent(new CustomEvent("formidable:context-reloaded"));
  } catch (err) {
    toast.error("workspace.collaboration.gigot.sync.pull.error", [backendErrMessage(err)]);
  } finally {
    pulling.value = false;
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
        <span v-else class="gigot-sync-muted">-</span>
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

    <FormSection
      v-if="hasPending"
      :title="t('workspace.collaboration.gigot.sync.message_title')"
    >
      <FormRow :label="t('workspace.collaboration.gigot.sync.message_label')">
        <TextareaField
          v-model="message"
          :placeholder="t('workspace.collaboration.gigot.sync.message_placeholder')"
          :rows="3"
        />
      </FormRow>
    </FormSection>

    <ProgressBar
      :active="inFlight"
      :label="progressLabel"
      :current="pulling ? progressCurrent : 0"
      :total="pulling ? progressTotal : 0"
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
        :disabled="!canPush"
        :title="pushDisabledHint"
        @click="doPush"
      >
        {{ pushing ? t('workspace.collaboration.gigot.sync.push.running') : t('workspace.collaboration.gigot.sync.push.button') }}
      </button>
      <button
        type="button"
        class="tool-btn"
        :disabled="!canPull"
        :title="pullDisabledHint"
        @click="doPull"
      >
        {{ pulling ? t('workspace.collaboration.gigot.sync.pull.running') : t('workspace.collaboration.gigot.sync.pull.button') }}
      </button>
      <button
        v-if="hasMirrors && hasMirrorAbility"
        type="button"
        class="tool-btn warning"
        :disabled="!canMirror"
        :title="mirrorDisabledHint"
        @click="doMirror"
      >
        {{ mirroring
          ? t('workspace.collaboration.gigot.sync.mirror.running')
          : destinations.length === 1
            ? t('workspace.collaboration.gigot.sync.mirror.button_one')
            : t('workspace.collaboration.gigot.sync.mirror.button_many', [String(destinations.length)]) }}
      </button>
    </div>
  </template>
</template>
