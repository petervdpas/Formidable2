<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

// Presentational repository-status panel. The owning view (Sync.vue)
// drives the fetch lifecycle so its sibling commit form can read the
// same status object — keeping the data flow one-directional.

type Status = {
  branch: string;
  tracking: string;
  detached: boolean;
  clean: boolean;
  modified: string[];
  untracked: string[];
  staged: string[];
  deleted: string[];
  renamed: string[];
  conflicted: string[];
};

const props = defineProps<{
  status: Status | null;
  loading: boolean;
  notARepo: boolean;
  errorMsg: string;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "discard", file: string): void;
}>();

const { t } = useI18n();

const buckets = computed(() => {
  const s = props.status;
  if (!s) return [];
  return [
    { key: "modified", files: s.modified },
    { key: "untracked", files: s.untracked },
    { key: "staged", files: s.staged },
    { key: "deleted", files: s.deleted },
    { key: "renamed", files: s.renamed },
    { key: "conflicted", files: s.conflicted },
  ].filter((b) => b.files.length > 0);
});

function bucketLabel(key: string, count: number): string {
  return t(`workspace.collaboration.status.bucket.${key}`, [count]);
}
</script>

<template>
  <div class="git-status">
    <div class="git-status-header">
      <h3 class="git-status-title">{{ t('workspace.collaboration.status.title') }}</h3>
      <button
        type="button"
        class="tool-btn"
        :disabled="loading"
        @click="emit('refresh')"
      >
        {{ t('workspace.collaboration.status.refresh') }}
      </button>
    </div>

    <p v-if="notARepo" class="section-warning">
      {{ t('workspace.collaboration.status.not_a_repo') }}
    </p>

    <p v-else-if="errorMsg" class="section-error">
      {{ t('workspace.collaboration.status.error', [errorMsg]) }}
    </p>

    <template v-else-if="status">
      <div class="git-status-meta">
        <span v-if="status.detached" class="badge badge-warn">
          {{ t('workspace.collaboration.status.detached') }}
        </span>
        <span v-else-if="status.branch" class="badge badge-accent">{{ status.branch }}</span>
        <span class="git-status-tracking">
          {{
            status.tracking
              ? t('workspace.collaboration.status.tracking', [status.tracking])
              : t('workspace.collaboration.status.tracking_none')
          }}
        </span>
        <span v-if="status.clean" class="badge badge-ok">
          {{ t('workspace.collaboration.status.clean') }}
        </span>
      </div>

      <details
        v-for="b in buckets"
        :key="b.key"
        class="git-status-bucket"
        open
      >
        <summary>{{ bucketLabel(b.key, b.files.length) }}</summary>
        <ul class="git-status-files">
          <li v-for="f in b.files" :key="f" class="git-status-file">
            <span class="git-status-file-path">{{ f }}</span>
            <button
              type="button"
              class="tool-btn"
              :disabled="loading"
              :title="t('workspace.collaboration.status.discard')"
              :aria-label="t('workspace.collaboration.status.discard')"
              @click="emit('discard', f)"
            >
              <i class="fa-solid fa-rotate-left" aria-hidden="true"></i>
            </button>
          </li>
        </ul>
      </details>
    </template>
  </div>
</template>
