import { onMounted, onUnmounted, ref, watch, type ComputedRef, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import { backendErrMessage } from "../utils/backendError";
import type { ModuleCache } from "./createModuleCache";

// useCommitGraph owns the lifecycle shared by every Commit Graph view
// in the collaboration workspace - irrespective of which backend
// fetches the commits:
//
//   - Read-through cache: on mount, render the cached value before
//     hitting the service. Survives component unmount because the
//     ModuleCache lives in a module-level binding.
//   - Race guard: only the latest in-flight fetch wins; obsolete
//     responses are discarded.
//   - journal:changed: when any backend records a sync hop, force a
//     refresh so the graph reflects post-pull/push state.
//   - Config-key watch: when the upstream key (gitRoot, gigot repo)
//     changes, invalidate the cache and fetch fresh.
//
// The composable stays backend-agnostic: callers pass a fetch
// callback, an emptyValue factory, and the ModuleCache instance. The
// per-backend Vue files supply the shape T and the service call;
// nothing git- or gigot-specific lives here. Updating cached state
// from consumer-driven mutations (e.g. git's lazy per-commit file
// load) goes through updateValue so the cache stays in lockstep.

export interface UseCommitGraphOptions<T> {
  /** Reactive cache discriminator: usually "<remote>|<repo>" or git_root. */
  cacheKey: ComputedRef<string>;
  /** Empty/cleared state - produced fresh on each call so the consumer
   *  can hold a mutable shape without shared-reference surprises. */
  emptyValue: () => T;
  /** Service call producing the new value. Throw on error; the
   *  composable catches and surfaces the message via errorMsg. */
  fetch: () => Promise<T>;
  /** Module-scoped cache instance - see createModuleCache. */
  cache: ModuleCache<T>;
  /** Optional error hook (toasts) - called after errorMsg is set so
   *  the consumer can fire a localized toast. */
  onError?: (err: unknown) => void;
}

export interface CommitGraphLifecycle<T> {
  value: Ref<T>;
  loading: Ref<boolean>;
  errorMsg: Ref<string>;
  /** Force=true bypasses the cache and re-fetches; false consults
   *  the cache and short-circuits on a hit. */
  load: (force: boolean) => Promise<void>;
  refresh: () => void;
  /** Mutate the cached value in place. Cache is updated in lockstep
   *  so a subsequent unmount/remount sees the latest state. */
  updateValue: (updater: (curr: T) => T) => void;
}

export function useCommitGraph<T>(opts: UseCommitGraphOptions<T>): CommitGraphLifecycle<T> {
  const value = ref<T>(opts.emptyValue()) as Ref<T>;
  const loading = ref(false);
  const errorMsg = ref("");
  let reqId = 0;
  let unsubscribe: (() => void) | null = null;

  async function load(force: boolean) {
    if (!force) {
      const hit = opts.cache.read(opts.cacheKey.value);
      if (hit !== null) {
        value.value = hit;
        return;
      }
    }
    const my = ++reqId;
    loading.value = true;
    errorMsg.value = "";
    try {
      const fresh = await opts.fetch();
      if (my !== reqId) return;
      value.value = fresh;
      opts.cache.write(opts.cacheKey.value, fresh);
    } catch (err) {
      if (my !== reqId) return;
      errorMsg.value = backendErrMessage(err);
      value.value = opts.emptyValue();
      opts.onError?.(err);
    } finally {
      if (my === reqId) loading.value = false;
    }
  }

  function refresh() {
    void load(true);
  }

  function updateValue(updater: (curr: T) => T) {
    const next = updater(value.value);
    value.value = next;
    opts.cache.write(opts.cacheKey.value, next);
  }

  onMounted(() => {
    void load(false);
    unsubscribe = Events.On("journal:changed", () => {
      void load(true);
    });
  });

  onUnmounted(() => {
    unsubscribe?.();
    unsubscribe = null;
  });

  watch(opts.cacheKey, () => {
    opts.cache.invalidate();
    void load(false);
  });

  return { value, loading, errorMsg, load, refresh, updateValue };
}
