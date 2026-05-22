// createModuleCache returns a tiny single-slot cache helper that
// holds one (key, value) pair behind a closure. Callers instantiate
// it at module scope (top-level inside a non-setup <script> block on
// a Vue SFC, or a top-level const in a plain .ts module). Because
// the returned object lives in the module's binding scope, its
// internal state survives component teardown - so navigating back
// to the section that owns the cache renders cached content
// instantly instead of re-fetching.
//
// Why this matters: variables declared inside `<script setup>` are
// compiled into setup() and are therefore per-instance, NOT
// per-module. A naive `let cachedValue = …` at the top of
// `<script setup>` re-initialises on every mount. This factory lets
// callers opt into true module-scoped caching from a non-setup
// <script> block without each one re-implementing the closure dance.
//
// Single-slot by design: if the key changes, the previous value is
// dropped. That matches what the Commit Graph views need (key =
// "<repo identity>"; one repo at a time). A multi-key cache would
// add eviction policy concerns this helper deliberately stays out of.
export interface ModuleCache<T> {
  read(key: string): T | null;
  write(key: string, value: T): void;
  invalidate(): void;
}

export function createModuleCache<T>(): ModuleCache<T> {
  let cachedKey = "";
  let cachedValue: T | null = null;
  return {
    read(key: string): T | null {
      if (cachedKey === key && cachedValue !== null) {
        return cachedValue;
      }
      return null;
    },
    write(key: string, value: T) {
      cachedKey = key;
      cachedValue = value;
    },
    invalidate() {
      cachedKey = "";
      cachedValue = null;
    },
  };
}
