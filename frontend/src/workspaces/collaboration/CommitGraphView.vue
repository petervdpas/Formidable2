<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import CommitGraphRow from "../../components/collaboration/CommitGraphRow.vue";
import CommitFileList from "../../components/collaboration/CommitFileList.vue";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import type {
  GraphCommit,
  ChangeFile,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git/models";
import { useConfig } from "../../composables/useConfig";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// CommitGraphView — workspace owns the fetch lifecycle (race-guarded
// like Sync.vue's status loader), passes commit nodes to the generic
// VisualGraph, fills its scoped slot with commit-row chrome (short
// hash, subject, author, ref pills, time). Auto-refreshes on
// journal:changed so the graph stays current after a pull/push.

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const gitRoot = computed(() => config.value?.git_root ?? "");
const commits = ref<GraphCommit[]>([]);
const loading = ref(false);
const errorMsg = ref("");

// Race guard — only the latest fetch wins.
let reqId = 0;

async function load() {
  const path = gitRoot.value.trim();
  if (path === "") {
    commits.value = [];
    errorMsg.value = "";
    return;
  }
  const my = ++reqId;
  loading.value = true;
  errorMsg.value = "";
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(path)) || path;
    const isRepo = await GitSvc.IsGitRepo(abs);
    if (my !== reqId) return;
    if (!isRepo) {
      commits.value = [];
      errorMsg.value = t("workspace.collaboration.status.not_a_repo");
      return;
    }
    const list = await GitSvc.LogGraph(abs, 100);
    if (my !== reqId) return;
    commits.value = (list ?? []) as GraphCommit[];
  } catch (err) {
    if (my !== reqId) return;
    errorMsg.value = backendErrMessage(err);
    toast.error("workspace.collaboration.graph.error", [backendErrMessage(err)]);
  } finally {
    if (my === reqId) loading.value = false;
  }
}

const nodes = computed<GraphNode<GraphCommit>[]>(() =>
  commits.value.map((c) => ({
    id: c.hash,
    parents: c.parents ?? [],
    data: c,
  })),
);

// Auto-refresh on journal events so a pull/push updates the graph
// without the user clicking Refresh.
let unsubscribe: (() => void) | null = null;

onMounted(async () => {
  await load();
  unsubscribe = Events.On("journal:changed", () => {
    void load();
  });
});

onUnmounted(() => {
  if (unsubscribe) unsubscribe();
});

// React to git_root switches.
watch(gitRoot, () => void load());

function refresh() {
  void load();
}

// Per-commit file list — lazy-loaded on first expand and cached so
// subsequent expands of the same row reuse the result. Refresh()
// drops the cache too since the underlying repo state may have
// shifted (e.g. after a pull).
const filesByHash = ref<Record<string, ChangeFile[] | "loading" | "error">>({});

async function loadCommitFiles(hash: string) {
  if (filesByHash.value[hash] && filesByHash.value[hash] !== "error") return;
  filesByHash.value = { ...filesByHash.value, [hash]: "loading" };
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const files = await GitSvc.CommitChanges(abs, hash);
    filesByHash.value = { ...filesByHash.value, [hash]: (files ?? []) as ChangeFile[] };
  } catch (err) {
    filesByHash.value = { ...filesByHash.value, [hash]: "error" };
    toast.error("workspace.collaboration.graph.error", [backendErrMessage(err)]);
  }
}

function onExpand(id: string) {
  void loadCommitFiles(id);
}

// Drop the file cache when the graph is reloaded — the same hashes
// can persist but their file content shouldn't be assumed stable
// (e.g. an amend rewrites; we'd rather re-fetch than show stale data).
watch(commits, () => {
  filesByHash.value = {};
});

</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.graph.info') }}</p>

  <div class="graph-toolbar">
    <button
      type="button"
      class="tool-btn"
      :disabled="loading"
      @click="refresh"
    >
      {{ loading ? t('common.loading') : t('common.refresh') }}
    </button>
  </div>

  <p v-if="errorMsg" class="muted small">{{ errorMsg }}</p>

  <p v-else-if="!loading && commits.length === 0" class="muted small">
    {{ t('workspace.collaboration.graph.empty') }}
  </p>

  <VisualGraph
    v-else
    :nodes="nodes"
    expandable
    @expand="onExpand"
  >
    <template #default="{ node }">
      <CommitGraphRow :commit="node.data" />
    </template>

    <template #details="{ node }">
      <CommitFileList :files="filesByHash[node.id]" />
    </template>
  </VisualGraph>
</template>
