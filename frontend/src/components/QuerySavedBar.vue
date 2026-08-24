<script setup lang="ts">
/**
 * QuerySavedBar - the saved-query row above the builder tabs: pick a stored
 * query and open it, name the current one and save it, or delete the one on
 * screen. Queries are grouped by where they live (on this machine vs shared
 * with the team through the synced storage tree), and the Save button says
 * which of the two it will write to. Presentational over the reactive object
 * from useSavedQueries; QueryDialog owns it and does the work.
 */
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { LOCATION_LABEL_KEYS, savedKey, type SavedQueries } from "../composables/useSavedQueries";

const props = defineProps<{ saved: SavedQueries; canSave: boolean }>();
const emit = defineEmits<{
  (e: "open", key: string): void;
  (e: "save"): void;
  (e: "delete", key: string): void;
}>();

const { t } = useI18n();

const locationLabel = (loc: string) => (LOCATION_LABEL_KEYS[loc] ? t(LOCATION_LABEL_KEYS[loc]) : loc);

// One optgroup per location, in the backend's order, empty ones dropped.
const groups = computed(() => {
  const seen: string[] = [];
  for (const q of props.saved.items) {
    const loc = q.location || "local";
    if (!seen.includes(loc)) seen.push(loc);
  }
  return seen.map((loc) => ({
    location: loc,
    label: locationLabel(loc),
    items: props.saved.items.filter((q) => (q.location || "local") === loc),
  }));
});

function openSelected() {
  if (props.saved.selectedKey) emit("open", props.saved.selectedKey);
}
function deleteSelected() {
  if (props.saved.selectedKey) emit("delete", props.saved.selectedKey);
}
</script>

<template>
  <div class="query-saved">
    <label class="query-saved-field">
      {{ t('query.saved') }}
      <select v-model="saved.selectedKey" :disabled="saved.items.length === 0">
        <option value="">{{ saved.items.length === 0 ? t('query.saved.none') : t('query.saved.pick') }}</option>
        <optgroup v-for="g in groups" :key="g.location" :label="g.label">
          <option v-for="q in g.items" :key="savedKey(q)" :value="savedKey(q)">{{ q.name }}</option>
        </optgroup>
      </select>
    </label>
    <button type="button" class="tool-btn" :disabled="!saved.selectedKey || saved.busy" @click="openSelected">
      {{ t('query.saved.open') }}
    </button>
    <button type="button" class="tool-btn danger" :disabled="!saved.selectedKey || saved.busy" @click="deleteSelected">
      {{ t('query.saved.delete') }}
    </button>
    <label class="query-saved-field query-saved-name">
      {{ t('query.saved.name') }}
      <input v-model="saved.name" type="text" :placeholder="t('query.saved.name_placeholder')" />
    </label>
    <button
      type="button"
      class="tool-btn"
      :disabled="!saved.canSave || !canSave || saved.busy"
      :title="t('query.saved.save_to', [locationLabel(saved.saveLocation)])"
      @click="emit('save')"
    >
      {{ t('query.saved.save') }}
    </button>
    <span class="query-saved-target form-description">{{ locationLabel(saved.saveLocation) }}</span>
  </div>
</template>
