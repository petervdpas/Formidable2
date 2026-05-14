<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { ChangeFile } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";

// Per-commit file diff for gigot's Commit Graph. Renders either the
// concrete changes array carried on the LogEntry, "loading" while a
// re-fetch is in flight, or "error" when the fetch failed. Pure
// presentational; parallel to GitCommitFileList on the git side and
// intentionally duplicated for per-backend separation.
defineProps<{
  files: ChangeFile[] | "loading" | "error" | undefined;
}>();

const { t } = useI18n();
</script>

<template>
  <p v-if="files === 'loading'" class="muted small commit-files-loading">
    {{ t('common.loading') }}
  </p>
  <p v-else-if="files === 'error'" class="muted small">
    {{ t('workspace.collaboration.graph.error', ['—']) }}
  </p>
  <ul v-else-if="Array.isArray(files)" class="commit-files">
    <li
      v-for="file in files"
      :key="file.path"
      class="commit-file"
    >
      <span :class="['commit-file-status', `commit-file-status--${file.status}`]">
        {{ file.status }}
      </span>
      <span class="commit-file-path">{{ file.path }}</span>
    </li>
    <li v-if="files.length === 0" class="muted small">
      {{ t('workspace.collaboration.graph.no_files') }}
    </li>
  </ul>
</template>
