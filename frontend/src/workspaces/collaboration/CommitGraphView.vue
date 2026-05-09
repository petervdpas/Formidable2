<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import type { GraphCommit } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git/models";
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

// Format helpers — pure presentational, kept local to this view.
function relativeTime(iso: string): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const diff = Math.max(0, Date.now() - t);
  const m = Math.round(diff / 60_000);
  if (m < 1) return t > 0 ? "just now" : iso;
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d}d ago`;
  const mo = Math.round(d / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.round(mo / 12)}y ago`;
}
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
    @node-click="(id) => void id /* future: open commit detail */"
  >
    <template #default="{ node }">
      <span class="commit-hash">{{ node.data?.short }}</span>
      <span class="commit-subject" :title="node.data?.subject">
        {{ node.data?.subject }}
      </span>
      <span
        v-for="ref in (node.data?.refs ?? [])"
        :key="ref"
        class="commit-ref-pill"
      >
        {{ ref }}
      </span>
      <span class="commit-author muted small">{{ node.data?.author }}</span>
      <span class="commit-time muted small">{{ relativeTime(node.data?.time ?? '') }}</span>
    </template>
  </VisualGraph>
</template>
