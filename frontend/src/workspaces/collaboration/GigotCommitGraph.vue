<script lang="ts">
// Module-level cache for the gigot Commit Graph - see createModuleCache
// for why a non-setup <script> block is required (per-module, not
// per-instance). Gigot's /log with_changes=true is server-side
// expensive (one per-commit diff-tree call per entry), so re-fetching
// on every re-entry of the section is noticeably slow.

import type { LogEntry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { createModuleCache } from "../../composables/createModuleCache";

const logCache = createModuleCache<LogEntry[]>();
</script>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import GigotCommitRow from "../../components/collaboration/GigotCommitRow.vue";
import GigotCommitFileList from "../../components/collaboration/GigotCommitFileList.vue";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import { useRemoteConfig } from "../../composables/useRemoteConfig";
import { useToast } from "../../composables/useToast";
import { useCommitGraph } from "../../composables/useCommitGraph";
import { backendErrMessage } from "../../utils/backendError";

// GigotCommitGraph wires useCommitGraph to the gigot backend. The
// shared lifecycle handles cache + race guard + journal:changed +
// onMounted refresh; this file owns only the fetch shape and the
// row chrome. Gigot's /log returns per-commit changes inline when
// asked with_changes=true, so expand/collapse needs no second
// round-trip - filesFor reads from the cached entries directly.

const { t } = useI18n();
const { gigotBaseURL: baseURL, gigotRepoName: repoName } = useRemoteConfig();
const toast = useToast();

const configured = computed(
  () => baseURL.value.trim() !== "" && repoName.value.trim() !== "",
);
const cacheKey = computed(() => `${baseURL.value.trim()}|${repoName.value.trim()}`);

const { value: entries, loading, errorMsg, refresh } = useCommitGraph<LogEntry[]>({
  cacheKey,
  emptyValue: () => [],
  cache: logCache,
  async fetch() {
    if (!configured.value) return [];
    const res = await GigotSvc.Log(100, true);
    return (res?.entries ?? []) as LogEntry[];
  },
  onError: (err) => toast.error("workspace.collaboration.graph.error", [backendErrMessage(err)]),
});

const nodes = computed<GraphNode<LogEntry>[]>(() =>
  entries.value.map((e) => ({
    id: e.hash,
    parents: e.parents ?? [],
    data: e,
  })),
);

function filesFor(hash: string) {
  const entry = entries.value.find((e) => e.hash === hash);
  return entry?.changes ?? [];
}
</script>

<template>
  <p class="section-info">{{ t('workspace.collaboration.graph.info') }}</p>

  <div v-if="!configured" class="gigot-graph-note">
    {{ t('workspace.collaboration.gigot.sync.not_configured') }}
  </div>

  <template v-else>
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

    <p v-else-if="!loading && entries.length === 0" class="muted small">
      {{ t('workspace.collaboration.graph.empty') }}
    </p>

    <VisualGraph
      v-else
      :nodes="nodes"
      expandable
    >
      <template #default="{ node }">
        <GigotCommitRow :entry="node.data" />
      </template>

      <template #details="{ node }">
        <GigotCommitFileList :files="filesFor(node.id)" />
      </template>
    </VisualGraph>
  </template>
</template>
