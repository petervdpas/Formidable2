<script setup lang="ts">
import { computed } from "vue";
import type { GraphCommit } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/git/models";

// GitCommitRow renders the per-commit header chrome: short hash,
// subject (with ellipsis), branch/HEAD pills, author, relative time.
// Pure presentational - props in, no events. Lives in
// components/collaboration/ alongside GitStatus.vue. Parallel to
// GigotCommitRow on the gigot side; intentionally duplicated for
// per-backend separation.
const props = defineProps<{
  commit: GraphCommit | undefined;
}>();

const refs = computed(() => props.commit?.refs ?? []);

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
  <span class="commit-hash">{{ commit?.short }}</span>
  <span class="commit-subject" :title="commit?.subject">
    {{ commit?.subject }}
  </span>
  <span
    v-for="ref in refs"
    :key="ref"
    class="commit-ref-pill"
  >
    {{ ref }}
  </span>
  <span class="commit-author muted small">{{ commit?.author }}</span>
  <span class="commit-time muted small">{{ relativeTime(commit?.time ?? '') }}</span>
</template>
