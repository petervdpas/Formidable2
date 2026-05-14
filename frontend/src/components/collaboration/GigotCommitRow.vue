<script setup lang="ts">
import { computed } from "vue";
import type { LogEntry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/collaboration/gigot/models";

// Per-commit header chrome for gigot's Commit Graph: short hash,
// subject (first line of message), branch/ref pills, author,
// relative time. Pure presentational. Parallel to CommitGraphRow on
// the git side; intentionally duplicated to keep gigot and git
// rendering paths independent (per the per-backend separation rule).
const props = defineProps<{
  entry: LogEntry | undefined;
}>();

const shortHash = computed(() => (props.entry?.hash ?? "").slice(0, 8));
const subject = computed(() => (props.entry?.message ?? "").split(/\r?\n/, 1)[0] ?? "");
const refs = computed(() => props.entry?.refs ?? []);

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
  <span class="commit-hash">{{ shortHash }}</span>
  <span class="commit-subject" :title="subject">
    {{ subject }}
  </span>
  <span
    v-for="ref in refs"
    :key="ref"
    class="commit-ref-pill"
  >
    {{ ref }}
  </span>
  <span class="commit-author muted small">{{ entry?.author }}</span>
  <span class="commit-time muted small">{{ relativeTime(entry?.date ?? '') }}</span>
</template>
