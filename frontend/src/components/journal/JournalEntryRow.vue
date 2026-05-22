<script setup lang="ts">
import type { Entry } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/journal/models";

// JournalEntryRow renders the per-entry header chrome inside
// VisualGraph's default slot. Three op families:
//   - file ops (create/update/delete/baseline): show op badge + path
//   - sync (RecordSync after a Push): backend pill + version short
//   - sync emitted from RecordRemoteSeen also shows up as op=sync;
//     the entry is structurally indistinguishable here, so we render
//     them the same way (the version is what matters either way).
//
// Pure presentational - props in, no events. Designed so other
// surfaces (a tray notification, a debug panel, etc.) can reuse it.
const props = defineProps<{
  entry: Entry | undefined;
}>();

function shortHash(v: string | undefined): string {
  if (!v) return "";
  return v.length > 7 ? v.slice(0, 7) : v;
}

function relativeTime(iso: string | undefined): string {
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
  <span :class="['journal-op', `journal-op--${props.entry?.op}`]">
    {{ props.entry?.op }}
  </span>

  <template v-if="props.entry?.op === 'sync'">
    <span class="journal-backend-pill">{{ props.entry?.backend }}</span>
    <span class="journal-version">{{ shortHash(props.entry?.version) }}</span>
    <span
      v-if="(props.entry?.pushed ?? 0) > 0 || (props.entry?.pulled ?? 0) > 0"
      class="muted small"
    >
      ↑{{ props.entry?.pushed ?? 0 }} ↓{{ props.entry?.pulled ?? 0 }}
    </span>
  </template>
  <template v-else>
    <span class="journal-path" :title="props.entry?.path">
      {{ props.entry?.path }}
    </span>
  </template>

  <span class="journal-time muted small">{{ relativeTime(props.entry?.ts) }}</span>
</template>
