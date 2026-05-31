import { ref, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import {
  Service as OpTrackSvc,
  type Status,
} from "../../bindings/github.com/petervdpas/formidable2/internal/optrack";

// useActiveOps reflects the backend op-tracker (internal/optrack) into the
// renderer. The backend registry is the single source of truth for what is
// running; this composable holds NO authoritative state, it mirrors the
// registry's snapshot. Buttons derive their disabled/running state from here
// instead of a local flag, so a page reload reflects an in-flight op (export,
// clone, reindex...) instead of looking idle and letting a second one start.
//
// Backend-driven: the registry emits `optrack:changed` carrying the current
// snapshot on every begin/end; we replace the list on each event and seed it
// once via Active() on first use. One module-level subscription serves every
// consumer (shared mirror, not per-component state).

const ops: Ref<Status[]> = ref([]);
let started = false;
let gotEvent = false;

function snapshotFrom(ev: unknown): Status[] {
  const data = (ev as { data?: unknown })?.data ?? ev;
  return Array.isArray(data) ? (data as Status[]) : [];
}

function ensureStarted(): void {
  if (started) return;
  started = true;
  Events.On("optrack:changed", (ev: unknown) => {
    gotEvent = true;
    ops.value = snapshotFrom(ev);
  });
  // Seed the current state on first use; an event that lands first wins, so a
  // stale initial fetch never clobbers a fresher snapshot.
  void OpTrackSvc.Active().then((list) => {
    if (!gotEvent) ops.value = list ?? [];
  });
}

export interface ActiveOps {
  /** Live mirror of the backend's in-flight ops. */
  ops: Ref<Status[]>;
  /** True while an op of exactly this kind is running. */
  isRunning: (kind: string) => boolean;
  /** True while any op whose kind starts with this prefix is running. */
  isRunningPrefix: (prefix: string) => boolean;
  /** The running op of this kind, for progress (current/total/label), or undefined. */
  op: (kind: string) => Status | undefined;
}

export function useActiveOps(): ActiveOps {
  ensureStarted();
  return {
    ops,
    isRunning: (kind) => ops.value.some((o) => o.kind === kind),
    isRunningPrefix: (prefix) => ops.value.some((o) => o.kind.startsWith(prefix)),
    op: (kind) => ops.value.find((o) => o.kind === kind),
  };
}
