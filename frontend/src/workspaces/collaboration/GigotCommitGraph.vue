<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import GigotCommitRow from "../../components/collaboration/GigotCommitRow.vue";
import GigotCommitFileList from "../../components/collaboration/GigotCommitFileList.vue";
import { Service as GigotSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot";
import type { LogEntry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";
import { useConfig } from "../../composables/useConfig";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// GigotCommitGraph — workspace owns the fetch lifecycle (race-guarded
// like GitSync.vue's status loader), passes commit nodes to the
// generic VisualGraph primitive, fills its scoped slot with
// gigot-specific commit-row chrome. Auto-refreshes on journal:changed
// so the graph stays current after a pull/push/sync.
//
// gigot's /log returns the per-commit changes inline when called with
// with_changes=true — so we fetch eagerly and drive expand/collapse
// from the cached entry instead of re-hitting the server. (git fetches
// per-commit lazily because its underlying backend serves them
// individually.)

const { t } = useI18n();
const { config } = useConfig();
const toast = useToast();

const baseURL = computed(() => config.value?.gigot_base_url ?? "");
const repoName = computed(() => config.value?.gigot_repo_name ?? "");
const configured = computed(
  () => baseURL.value.trim() !== "" && repoName.value.trim() !== "",
);

const entries = ref<LogEntry[]>([]);
const loading = ref(false);
const errorMsg = ref("");

// Race guard — only the latest fetch wins.
let reqId = 0;

async function load() {
  if (!configured.value) {
    entries.value = [];
    errorMsg.value = "";
    return;
  }
  const my = ++reqId;
  loading.value = true;
  errorMsg.value = "";
  try {
    const res = await GigotSvc.Log(100, true);
    if (my !== reqId) return;
    entries.value = (res?.entries ?? []) as LogEntry[];
  } catch (err) {
    if (my !== reqId) return;
    errorMsg.value = backendErrMessage(err);
    entries.value = [];
    toast.error("workspace.collaboration.graph.error", [errorMsg.value]);
  } finally {
    if (my === reqId) loading.value = false;
  }
}

const nodes = computed<GraphNode<LogEntry>[]>(() =>
  entries.value.map((e) => ({
    id: e.hash,
    parents: e.parents ?? [],
    data: e,
  })),
);

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

watch(
  () => [baseURL.value, repoName.value] as const,
  () => void load(),
);

function refresh() {
  void load();
}

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
