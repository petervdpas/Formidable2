<script lang="ts">
// Module-level cache for the git Commit Graph - see createModuleCache
// for why a non-setup <script> block is required (per-module, not
// per-instance). Cached pair: the commits list AND the lazily-loaded
// per-commit file map, so re-entering the view preserves which rows
// the user has already expanded.

import type {
  GraphCommit,
  ChangeFile,
} from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git/models";
import { createModuleCache } from "../../composables/createModuleCache";

type FilesCache = Record<string, ChangeFile[] | "loading" | "error">;

interface GitGraphValue {
  commits: GraphCommit[];
  files: FilesCache;
}

const graphCache = createModuleCache<GitGraphValue>();
</script>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import GitCommitRow from "../../components/collaboration/GitCommitRow.vue";
import GitCommitFileList from "../../components/collaboration/GitCommitFileList.vue";
import { Service as GitSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git";
import { Service as SystemSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/system";
import { useConfig } from "../../composables/useConfig";
import { useToast } from "../../composables/useToast";
import { useCommitGraph } from "../../composables/useCommitGraph";
import { backendErrMessage } from "../../utils/backendError";

// GitCommitGraph wires useCommitGraph to the git backend. The shared
// lifecycle handles cache + race guard + journal:changed + onMounted
// refresh; this file owns only the fetch shape and the row chrome.
// "Not a git repo" is a non-error state - fetch returns the empty
// shape and toggles notARepo so the template can render the right
// message without going through errorMsg / toast.

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const gitRoot = computed(() => config.value?.git_root ?? "");
const cacheKey = computed(() => gitRoot.value.trim());
const notARepo = ref(false);

const { value, loading, errorMsg, refresh, updateValue } = useCommitGraph<GitGraphValue>({
  cacheKey,
  emptyValue: () => ({ commits: [], files: {} }),
  cache: graphCache,
  async fetch() {
    const path = gitRoot.value.trim();
    if (path === "") {
      notARepo.value = false;
      return { commits: [], files: {} };
    }
    const abs = (await SystemSvc.ResolveAbsolutePath(path)) || path;
    const isRepo = await GitSvc.IsGitRepo(abs);
    if (!isRepo) {
      notARepo.value = true;
      return { commits: [], files: {} };
    }
    notARepo.value = false;
    const list = await GitSvc.LogGraph(abs, 100);
    return { commits: (list ?? []) as GraphCommit[], files: {} };
  },
  onError: (err) => toast.error("workspace.collaboration.graph.error", [backendErrMessage(err)]),
});

const commits = computed(() => value.value.commits);
const filesByHash = computed(() => value.value.files);

const nodes = computed<GraphNode<GraphCommit>[]>(() =>
  commits.value.map((c) => ({
    id: c.hash,
    parents: c.parents ?? [],
    data: c,
  })),
);

// Per-commit file list - lazy-loaded on first expand. updateValue
// rewrites the cached shape in lockstep so re-entry preserves the
// expanded rows the user has already opened.
async function loadCommitFiles(hash: string) {
  const curr = filesByHash.value[hash];
  if (curr && curr !== "error") return;
  updateValue((v) => ({ ...v, files: { ...v.files, [hash]: "loading" } }));
  try {
    const abs = (await SystemSvc.ResolveAbsolutePath(gitRoot.value)) || gitRoot.value;
    const files = await GitSvc.CommitChanges(abs, hash);
    updateValue((v) => ({
      ...v,
      files: { ...v.files, [hash]: (files ?? []) as ChangeFile[] },
    }));
  } catch (err) {
    updateValue((v) => ({ ...v, files: { ...v.files, [hash]: "error" } }));
    toast.error("workspace.collaboration.graph.error", [backendErrMessage(err)]);
  }
}

function onExpand(id: string) {
  void loadCommitFiles(id);
}

const notARepoMsg = computed(() =>
  notARepo.value ? t("workspace.collaboration.status.not_a_repo") : "",
);
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

  <p v-if="notARepo" class="muted small">{{ notARepoMsg }}</p>

  <p v-else-if="errorMsg" class="muted small">{{ errorMsg }}</p>

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
      <GitCommitRow :commit="node.data" />
    </template>

    <template #details="{ node }">
      <GitCommitFileList :files="filesByHash[node.id]" />
    </template>
  </VisualGraph>
</template>
