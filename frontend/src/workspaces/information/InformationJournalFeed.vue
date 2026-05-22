<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Events } from "@wailsio/runtime";
import VisualGraph, { type GraphNode } from "../../components/VisualGraph.vue";
import JournalEntryRow from "../../components/journal/JournalEntryRow.vue";
import JournalEntryDetails from "../../components/journal/JournalEntryDetails.vue";
import { Service as JournalSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/journal";
import type { Entry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/journal/models";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

// InformationJournalFeed reuses VisualGraph to render the journal's
// .changes.log as a chronological timeline. Each entry becomes a node
// with parents = [previous entry's id] (linear chain), so the lane
// renders as one continuous line and adjacent rows visibly connect.
//
// Smart/dumb split: this workspace owns fetch + journal:changed
// subscription; VisualGraph + JournalEntryRow + JournalEntryDetails
// are pure presentational pieces.

const { t } = useI18n();
const toast = useToast();

const entries = ref<Entry[]>([]);
const loading = ref(false);

let reqId = 0;

async function load() {
  const my = ++reqId;
  loading.value = true;
  try {
    const list = await JournalSvc.RecentEntries(100);
    if (my !== reqId) return;
    entries.value = (list ?? []) as Entry[];
  } catch (err) {
    if (my !== reqId) return;
    toast.error("workspace.information.journal_feed.error", [backendErrMessage(err)]);
  } finally {
    if (my === reqId) loading.value = false;
  }
}

// Map entries → GraphNodes. Each entry needs a stable unique id; the
// JSONL log doesn't carry one, so we synthesize from index + ts.
// `parents` is the previous entry's id - a strict linear chain.
const nodes = computed<GraphNode<Entry>[]>(() =>
  entries.value.map((e, i) => {
    const id = `${i}|${e.ts}`;
    const parents = i < entries.value.length - 1
      ? [`${i + 1}|${entries.value[i + 1].ts}`]
      : [];
    return { id, parents, data: e };
  }),
);

// Auto-refresh on journal:changed so any mutation / sync update the
// feed without the user clicking Refresh.
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

function refresh() {
  void load();
}
</script>

<template>
  <p class="section-info">{{ t('workspace.information.journal_feed.info') }}</p>

  <div class="journal-feed-toolbar">
    <button
      type="button"
      class="tool-btn"
      :disabled="loading"
      @click="refresh"
    >
      {{ loading ? t('common.loading') : t('common.refresh') }}
    </button>
  </div>

  <p v-if="!loading && entries.length === 0" class="muted small">
    {{ t('workspace.information.journal_feed.empty') }}
  </p>

  <VisualGraph
    v-else
    :nodes="nodes"
    expandable
  >
    <template #default="{ node }">
      <JournalEntryRow :entry="node.data" />
    </template>

    <template #details="{ node }">
      <JournalEntryDetails :entry="node.data" />
    </template>
  </VisualGraph>
</template>
