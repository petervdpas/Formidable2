<script setup lang="ts">
/*
 * StatusGitQuick — footer git status indicator + jump to Sync.
 *
 * Shows: branch icon, optional ↑N (ahead) and ↓M (behind) counts, and
 * a `*` when the working tree is dirty. Click → switches the ribbon to
 * Collaboration and the sub-section to "git-sync".
 *
 * Refresh: load on mount, on git_root change, and on the global
 * `journal:changed` event (the git module's commit/fetch/push/pull
 * paths all emit a journal entry, so this is the same trigger Sync /
 * CommitGraphView use to stay current — no second poller).
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

// Reqid pattern mirrors Sync.vue — guards against an in-flight load
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

onMounted(() => {
  void load();
  unsubscribe = Events.On("journal:changed", () => { void load(); });
});

onBeforeUnmount(() => {
  if (unsubscribe) unsubscribe();
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
