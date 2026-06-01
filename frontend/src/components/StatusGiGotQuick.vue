<script setup lang="ts">
/*
 * StatusGiGotQuick - footer Gigot status indicator + jump to Sync.
 *
 * Mirrors StatusGitQuick so the two backends present the same footer
 * affordance: identical icon, identical position, similar indicators,
 * one-click jump to the matching Sync workspace. Differences vs Git:
 *   - "ahead" doesn't apply (Gigot has no separate local commit step),
 *     so we only render a `*` for pending changes and a `↓` when the
 *     remote ledger has moved past the local one.
 *   - LedgerSummary is local-only (no HTTP); Head() is the remote probe.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { useRemoteConfig } from "../composables/useRemoteConfig";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useCollaborationSection } from "../composables/useCollaborationSection";
import { confirmLeave } from "../composables/useNavGuard";
import { Service as GigotSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import type {
  LedgerSummary,
  HeadResponse,
  Destination,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";

const { t } = useI18n();
const { contextFolder, gigotBaseURL: baseURL, gigotRepoName: repoName } = useRemoteConfig();
const { setActive: setWorkspace } = useActiveWorkspace();
const { setActive: setSection } = useCollaborationSection();

const configured = computed(
  () =>
    contextFolder.value.trim() !== ""
    && baseURL.value.trim() !== ""
    && repoName.value.trim() !== "",
);

const summary = ref<LedgerSummary | null>(null);
const head = ref<HeadResponse | null>(null);
const destinations = ref<Destination[]>([]);

let reqId = 0;
async function load() {
  const my = ++reqId;
  if (!configured.value) {
    summary.value = null;
    head.value = null;
    destinations.value = [];
    return;
  }
  try {
    const s = await GigotSvc.LedgerSummary();
    if (my !== reqId) return;
    summary.value = (s as LedgerSummary | null) ?? null;
  } catch {
    if (my !== reqId) return;
    summary.value = null;
  }
  try {
    const h = await GigotSvc.Head();
    if (my !== reqId) return;
    head.value = (h as HeadResponse | null) ?? null;
  } catch {
    if (my !== reqId) return;
    head.value = null;
  }
  try {
    const ds = await GigotSvc.Destinations();
    if (my !== reqId) return;
    destinations.value = (ds as Destination[] | null) ?? [];
  } catch {
    if (my !== reqId) return;
    destinations.value = [];
  }
}

let unsubscribe: (() => void) | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;
const POLL_MS = 30_000;

function onVisibility() {
  if (document.visibilityState === "visible") void load();
}
function onSyncRefreshed() { void load(); }

onMounted(() => {
  void load();
  unsubscribe = Events.On("journal:changed", () => { void load(); });
  pollTimer = setInterval(() => { void load(); }, POLL_MS);
  document.addEventListener("visibilitychange", onVisibility);
  window.addEventListener("formidable:gigot-refreshed", onSyncRefreshed);
});

onBeforeUnmount(() => {
  if (unsubscribe) unsubscribe();
  if (pollTimer) clearInterval(pollTimer);
  document.removeEventListener("visibilitychange", onVisibility);
  window.removeEventListener("formidable:gigot-refreshed", onSyncRefreshed);
});

watch([contextFolder, baseURL, repoName], () => { void load(); });

const pendingCount = computed(() => {
  const s = summary.value;
  if (!s) return 0;
  return (s.changed?.length ?? 0) + (s.deleted?.length ?? 0);
});
const dirty = computed(() => pendingCount.value > 0);
const behind = computed(() => {
  const local = summary.value?.version ?? "";
  const remote = head.value?.version ?? "";
  if (!local || !remote) return false;
  return local !== remote;
});
const mirrorLag = computed(
  () => destinations.value.filter(d => d.remote_status !== "in_sync").length,
);

const tooltip = computed(() => {
  if (!configured.value) return t("statusbar.gigotquick.not_configured");
  const parts: string[] = [];
  if (repoName.value) parts.push(repoName.value);
  parts.push(
    dirty.value
      ? t("statusbar.gigotquick.dirty", [pendingCount.value])
      : t("statusbar.gigotquick.clean"),
  );
  if (mirrorLag.value > 0) {
    parts.push(t("statusbar.gigotquick.mirror_lag", [mirrorLag.value]));
  }
  if (behind.value) parts.push(t("statusbar.gigotquick.behind"));
  return parts.join(" • ");
});

async function onClick() {
  if (!(await confirmLeave())) return; // honor an unsaved-changes guard
  setWorkspace("collaboration");
  setSection("gigot-sync");
}
</script>

<template>
  <button
    type="button"
    class="status-gigotquick"
    :title="tooltip"
    :aria-label="tooltip"
    @click="onClick"
  >
    <i class="fa-solid fa-code-branch" aria-hidden="true"></i>
    <span v-if="mirrorLag > 0" class="status-gigotquick-mirror">↑{{ mirrorLag }}</span>
    <span v-if="behind" class="status-gigotquick-behind">↓</span>
    <span v-if="dirty" class="status-gigotquick-dirty" aria-hidden="true">*</span>
  </button>
</template>
