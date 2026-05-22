<script setup lang="ts">
/*
 * StatusGitQuick - footer git status indicator + jump to Sync.
 *
 * Shows: branch icon, optional ↑N (ahead) and ↓M (behind) counts, and
 * a `*` when the working tree is dirty. Click → switches the ribbon to
 * Collaboration and the sub-section to "git-sync".
 *
 * Refresh: load on mount, on git_root change, and on the global
 * `journal:changed` event (the git module's commit/fetch/push/pull
 * paths all emit a journal entry, so this is the same trigger Sync /
 * GitCommitGraph use to stay current - no second poller).
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import { useConfig } from "../composables/useConfig";
import { useActiveWorkspace } from "../composables/useActiveWorkspace";
import { useCollaborationSection } from "../composables/useCollaborationSection";
import { Service as GitSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import type { Status } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/system";

const { t } = useI18n();
const { config } = useConfig();
const { setActive: setWorkspace } = useActiveWorkspace();
const { setActive: setSection } = useCollaborationSection();

const gitRoot = computed(() => config.value?.git_root ?? "");
const status = ref<Status | null>(null);
const isRepo = ref(false);

// Reqid pattern mirrors Sync.vue - guards against an in-flight load
// resolving after the user changed git_root mid-fetch.
let reqId = 0;
async function load() {
  const my = ++reqId;
  const path = gitRoot.value.trim();
  if (!path) {
    isRepo.value = false;
    status.value = null;
    return;
  }
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(path)) || path;
    const ok = await GitSvc.IsGitRepo(abs);
    if (my !== reqId) return;
    if (!ok) {
      isRepo.value = false;
      status.value = null;
      return;
    }
    const s = await GitSvc.Status(abs);
    if (my !== reqId) return;
    isRepo.value = true;
    status.value = (s as Status | null) ?? null;
  } catch {
    if (my !== reqId) return;
    isRepo.value = false;
    status.value = null;
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
  // In-app git ops (commit/fetch/push/pull) emit journal entries, so
  // this catches everything done through Formidable's own UI.
  unsubscribe = Events.On("journal:changed", () => { void load(); });
  // External changes (terminal git, IDE save, another tool) bypass
  // the journal - a slow poll is the cheapest way to keep the footer
  // honest. GitSvc.Status is a single fs scan; 30s is plenty.
  pollTimer = setInterval(() => { void load(); }, POLL_MS);
  // Catch the "switched away to terminal, came back" case faster than
  // the poll: refresh whenever the tab/window becomes visible again.
  document.addEventListener("visibilitychange", onVisibility);
  // Sync.vue dispatches this after every successful status read so a
  // manual Refresh click in the Sync page propagates to the footer
  // immediately instead of waiting for the poll to catch up.
  window.addEventListener("formidable:git-refreshed", onSyncRefreshed);
});

onBeforeUnmount(() => {
  if (unsubscribe) unsubscribe();
  if (pollTimer) clearInterval(pollTimer);
  document.removeEventListener("visibilitychange", onVisibility);
  window.removeEventListener("formidable:git-refreshed", onSyncRefreshed);
});

watch(gitRoot, () => { void load(); });

const ahead = computed(() => status.value?.ahead ?? 0);
const behind = computed(() => status.value?.behind ?? 0);
const dirty = computed(() => status.value !== null && !status.value.clean);
const branch = computed(() => status.value?.branch ?? "");

const tooltip = computed(() => {
  if (!isRepo.value) return t("statusbar.gitquick.not_a_repo");
  const parts: string[] = [];
  if (branch.value) parts.push(branch.value);
  parts.push(t("statusbar.gitquick.ahead_behind", [ahead.value, behind.value]));
  parts.push(
    dirty.value
      ? t("statusbar.gitquick.dirty")
      : t("statusbar.gitquick.clean"),
  );
  return parts.join(" • ");
});

function onClick() {
  setWorkspace("collaboration");
  setSection("git-sync");
}
</script>

<template>
  <button
    type="button"
    class="status-gitquick"
    :title="tooltip"
    :aria-label="tooltip"
    @click="onClick"
  >
    <i class="fa-solid fa-code-branch" aria-hidden="true"></i>
    <span v-if="ahead > 0" class="status-gitquick-ahead">↑{{ ahead }}</span>
    <span v-if="behind > 0" class="status-gitquick-behind">↓{{ behind }}</span>
    <span v-if="dirty" class="status-gitquick-dirty" aria-hidden="true">*</span>
  </button>
</template>
