<script setup lang="ts">
import { useI18n } from "vue-i18n";
import type { ChangeFile } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git/models";

// GitCommitFileList renders a commit's file diff. Accepts either a
// concrete files array (loaded), the literal string "loading"
// (placeholder), or "error" (fetch failed). The three-state shape
// matches GitCommitGraph's per-hash cache so the workspace can hand
// its state down without translation.
//
// Pure presentational. Parallel to GigotCommitFileList on the gigot
// side; intentionally duplicated for per-backend separation.
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
    {{ t('workspace.collaboration.graph.error', ['-']) }}
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
