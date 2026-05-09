<script setup lang="ts">
import type { Entry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/journal/models";

// JournalEntryDetails renders the full metadata for a journal entry
// inside VisualGraph's `details` slot. Mirrors gigot's JSONL line
// shape so the user can reconcile UI rows against the on-disk log
// when they need to.
defineProps<{
  entry: Entry | undefined;
}>();
</script>

<template>
  <dl class="journal-details">
    <template v-if="entry?.op === 'sync'">
      <dt>backend</dt>
      <dd>{{ entry?.backend }}</dd>
      <dt>version</dt>
      <dd class="journal-mono">{{ entry?.version }}</dd>
      <dt>pushed</dt>
      <dd>{{ entry?.pushed ?? 0 }}</dd>
      <dt>pulled</dt>
      <dd>{{ entry?.pulled ?? 0 }}</dd>
    </template>
    <template v-else>
      <dt>path</dt>
      <dd class="journal-mono">{{ entry?.path }}</dd>
      <dt v-if="(entry?.bytes ?? 0) > 0">bytes</dt>
      <dd v-if="(entry?.bytes ?? 0) > 0">{{ entry?.bytes }}</dd>
    </template>
    <dt>ts</dt>
    <dd class="journal-mono">{{ entry?.ts }}</dd>
  </dl>
</template>
