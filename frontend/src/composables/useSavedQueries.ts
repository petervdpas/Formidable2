// useSavedQueries owns the saved-query lifecycle for one template: the list,
// the selection, and the save/open/delete calls. QueryDialog creates it beside
// useQueryBuilder and hands the reactive object to QuerySavedBar, which is
// presentational. The backend owns the records (id derivation, timestamps,
// validation, the two on-disk roots and which one a new save lands in); this
// only transports them.

import { computed, ref, reactive, type Ref } from "vue";
import {
  Service as QuerySvc,
  SavedQuery,
  type Spec,
} from "../../bindings/github.com/petervdpas/formidable2/internal/modules/query";

// Explicit key map for the backend's closed location set (never interpolate a
// lookup key). A location the backend adds later falls back to its raw value.
export const LOCATION_LABEL_KEYS: Record<string, string> = {
  local: "query.saved.location.local",
  shared: "query.saved.location.shared",
};

// A record is addressed by location + id: the same id can exist in both roots
// (a local copy of a query a peer shared), so the select needs both.
export function savedKey(q: { id: string; location?: string }): string {
  return `${q.location || "local"}:${q.id}`;
}

export function useSavedQueries(templateFilename: Ref<string>) {
  const items = ref<SavedQuery[]>([]);
  const selectedKey = ref("");
  const name = ref("");
  const busy = ref(false);
  const defaultLocation = ref("local");

  const selected = computed(() => items.value.find((q) => savedKey(q) === selectedKey.value) ?? null);
  const canSave = computed(() => name.value.trim().length > 0);
  // Where the Save button will put it: the record's own root when re-saving
  // one that is already stored, otherwise the profile setting.
  const saveLocation = computed(() => selected.value?.location || defaultLocation.value);

  async function refresh() {
    items.value = (await QuerySvc.ListSavedQueries(templateFilename.value)) ?? [];
    if (selectedKey.value && !items.value.some((q) => savedKey(q) === selectedKey.value)) {
      selectedKey.value = "";
    }
    defaultLocation.value = await QuerySvc.DefaultSavedQueryLocation();
  }

  function clear() {
    items.value = [];
    selectedKey.value = "";
    name.value = "";
  }

  // open returns the stored spec so the caller can feed it to the builder;
  // it also adopts the record's name, so a re-save overwrites in place.
  async function open(key: string): Promise<Spec> {
    const row = items.value.find((q) => savedKey(q) === key);
    if (!row) throw new Error("query: no such saved query");
    busy.value = true;
    try {
      const q = await QuerySvc.GetSavedQuery(templateFilename.value, row.id, row.location ?? "");
      selectedKey.value = savedKey(q);
      name.value = q.name;
      return q.spec;
    } finally {
      busy.value = false;
    }
  }

  // save writes the current spec under the typed name. Id and location only
  // travel when the name still matches the selected record: renaming makes a
  // new saved query rather than silently retitling the old one, and an empty
  // location lets the backend apply the profile setting.
  async function save(spec: Spec): Promise<SavedQuery> {
    busy.value = true;
    try {
      const keep = selected.value && selected.value.name === name.value.trim() ? selected.value : null;
      const stored = await QuerySvc.SaveQuery(
        SavedQuery.createFrom({
          id: keep?.id ?? "",
          name: name.value.trim(),
          template: templateFilename.value,
          location: keep?.location ?? "",
          spec,
        }),
      );
      await refresh();
      selectedKey.value = savedKey(stored);
      name.value = stored.name;
      return stored;
    } finally {
      busy.value = false;
    }
  }

  async function remove(key: string) {
    const row = items.value.find((q) => savedKey(q) === key);
    if (!row) return;
    busy.value = true;
    try {
      await QuerySvc.DeleteSavedQuery(templateFilename.value, row.id, row.location ?? "");
      if (selectedKey.value === key) {
        selectedKey.value = "";
        name.value = "";
      }
      await refresh();
    } finally {
      busy.value = false;
    }
  }

  return reactive({
    items,
    selectedKey,
    name,
    busy,
    defaultLocation,
    selected,
    canSave,
    saveLocation,
    refresh,
    clear,
    open,
    save,
    remove,
  });
}

export type SavedQueries = ReturnType<typeof useSavedQueries>;
