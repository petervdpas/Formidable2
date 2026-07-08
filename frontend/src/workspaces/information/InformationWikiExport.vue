<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { SwitchField, TextField } from "../../components/fields";
import { ExportService as WikiExportSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/wiki";
import { Service as FormSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import type { DeckOption } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/form";
import { useTemplates } from "../../composables/useTemplates";
import { useDialog } from "../../composables/useDialog";
import { useToast } from "../../composables/useToast";
import { backendErrMessage } from "../../utils/backendError";

const { t } = useI18n();
const { filenames, cache } = useTemplates();
const { chooseSaveFile } = useDialog();
const toast = useToast();

// Selection state, held locally (not persisted). selectedTemplates is the set of
// chosen template filenames; selectedDecks narrows a presentation to specific
// slideset values; decksFor caches each presentation's available decks.
const selectedTemplates = ref<Set<string>>(new Set());
const selectedDecks = ref<Map<string, Set<string>>>(new Map());
const decksFor = ref<Map<string, DeckOption[]>>(new Map());
const exporting = ref(false);
const search = ref("");

// Dependency closure: templates the current picks link to (via relations and api
// fields) are pulled into the bundle so every link resolves to a page inside the
// zip. The backend resolves it; the frontend force-toggles the extras on and
// explains why. autoIncluded is the set of auto-added filenames, becauseMap says
// which picks required each, missingDeps lists dangling references left out.
const autoIncluded = ref<Set<string>>(new Set());
const becauseMap = ref<Record<string, string[] | undefined>>({});
const missingDeps = ref<string[]>([]);

watch(
  selectedTemplates,
  async (picks) => {
    const seeds = [...picks];
    if (seeds.length === 0) {
      autoIncluded.value = new Set();
      becauseMap.value = {};
      missingDeps.value = [];
      return;
    }
    try {
      const res = await WikiExportSvc.ResolveDependencies(seeds);
      autoIncluded.value = new Set(res.added ?? []);
      becauseMap.value = res.because ?? {};
      missingDeps.value = res.missing ?? [];
    } catch {
      // Leave the explicit picks working even if resolution fails; the backend
      // still expands the closure at export time.
      autoIncluded.value = new Set();
      becauseMap.value = {};
      missingDeps.value = [];
    }
  },
  { deep: true },
);

// A row is "forced" when it is pulled in by a dependency but not picked directly:
// its switch shows on and locked. isRowOn covers both explicit and forced.
function isForced(fn: string): boolean {
  return autoIncluded.value.has(fn) && !selectedTemplates.value.has(fn);
}
function isRowOn(fn: string): boolean {
  return selectedTemplates.value.has(fn) || autoIncluded.value.has(fn);
}
function forcedBy(fn: string): string {
  return (becauseMap.value[fn] ?? []).map(displayName).join(", ");
}

function displayName(filename: string): string {
  return cache.value.get(filename)?.name?.trim() || filename;
}
function isPresentation(filename: string): boolean {
  return cache.value.get(filename)?.presentation === true;
}
function decksAvailable(filename: string): DeckOption[] {
  return decksFor.value.get(filename) ?? [];
}

// Fetch each presentation's decks once the template cache reveals it is one, so
// the picker can render its slideset sub-toggles synchronously.
watch(
  [filenames, cache],
  async () => {
    for (const fn of filenames.value) {
      if (isPresentation(fn) && !decksFor.value.has(fn)) {
        let decks: DeckOption[] = [];
        try {
          // Only decks that actually contain slides (never offer an empty deck).
          decks = (await FormSvc.PlayableDecks(fn)) ?? [];
        } catch {
          decks = [];
        }
        decksFor.value = new Map(decksFor.value).set(fn, decks);
      }
    }
  },
  { immediate: true, deep: true },
);

const visibleFilenames = computed<string[]>(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return filenames.value;
  return filenames.value.filter(
    (f) => f.toLowerCase().includes(q) || displayName(f).toLowerCase().includes(q),
  );
});

function isTemplateSelected(fn: string): boolean {
  return selectedTemplates.value.has(fn);
}
function setTemplateSelected(fn: string, on: boolean) {
  const next = new Set(selectedTemplates.value);
  if (on) {
    next.add(fn);
    // Selecting a presentation defaults to all of its decks.
    const avail = decksAvailable(fn);
    if (isPresentation(fn) && avail.length) {
      selectedDecks.value = new Map(selectedDecks.value).set(
        fn,
        new Set(avail.map((d) => d.value)),
      );
    }
  } else {
    next.delete(fn);
  }
  selectedTemplates.value = next;
}

function isDeckSelected(fn: string, value: string): boolean {
  return selectedDecks.value.get(fn)?.has(value) ?? false;
}
function setDeckSelected(fn: string, value: string, on: boolean) {
  const map = new Map(selectedDecks.value);
  const set = new Set(map.get(fn) ?? []);
  if (on) set.add(value);
  else set.delete(value);
  map.set(fn, set);
  selectedDecks.value = map;
}

// The backend request: filename -> deck values. Empty array = all decks of a
// presentation / a document (backend distinguishes by the template kind). A
// presentation with decks available but none chosen is excluded entirely.
const selections = computed<Record<string, string[]>>(() => {
  const out: Record<string, string[]> = {};
  const include = (fn: string) => {
    if (!isPresentation(fn)) {
      out[fn] = [];
      return;
    }
    const avail = decksAvailable(fn);
    if (avail.length === 0) {
      out[fn] = []; // single-deck presentation
      return;
    }
    // A forced presentation carries all its decks (the reader gets the whole
    // thing, since they did not narrow it themselves).
    if (isForced(fn)) {
      out[fn] = avail.map((d) => d.value);
      return;
    }
    const chosen = [...(selectedDecks.value.get(fn) ?? [])].filter((v) =>
      avail.some((d) => d.value === v),
    );
    if (chosen.length === 0) return; // selected but no deck picked -> skip
    out[fn] = chosen;
  };
  for (const fn of selectedTemplates.value) include(fn);
  // Auto-included templates join the payload too, so it matches what the UI
  // shows. The backend also expands the closure, so this stays consistent.
  for (const fn of autoIncluded.value) if (!(fn in out)) include(fn);
  return out;
});

// Count explicit picks that actually contribute (a presentation selected with no
// deck picked does not). Auto-added templates are reported separately.
const selectedCount = computed(() => {
  let n = 0;
  for (const fn of selectedTemplates.value) if (fn in selections.value) n++;
  return n;
});
const autoCount = computed(() => autoIncluded.value.size);
const canExport = computed(() => selectedCount.value > 0 && !exporting.value);

async function doExport() {
  if (!canExport.value) return;
  const path = await chooseSaveFile("wiki-export.zip", [
    { displayName: "Zip archive", pattern: "*.zip" },
  ]);
  if (!path) return;
  exporting.value = true;
  try {
    const skipped = await WikiExportSvc.ExportBundle(selections.value, path);
    toast.success("workspace.information.wiki_export.toast_success");
    if (skipped && skipped.length) {
      toast.warn("workspace.information.wiki_export.toast_skipped", [skipped.join(", ")]);
    }
  } catch (e) {
    toast.error("workspace.information.wiki_export.toast_error", [backendErrMessage(e)]);
  } finally {
    exporting.value = false;
  }
}
</script>

<template>
  <p class="section-info">{{ t('workspace.information.wiki_export.info') }}</p>
  <p class="muted small">{{ t('workspace.information.wiki_export.note_decks') }}</p>
  <p class="muted small">{{ t('workspace.information.wiki_export.note_related') }}</p>

  <div class="wiki-export-search">
    <TextField
      v-model="search"
      type="text"
      :placeholder="t('workspace.information.wiki_export.search_placeholder')"
      clearable
    />
  </div>

  <p v-if="filenames.length === 0" class="muted small">
    {{ t('workspace.information.wiki_export.loading') }}
  </p>
  <p v-else-if="visibleFilenames.length === 0" class="muted small">
    {{ t('workspace.information.wiki_export.no_templates_found') }}
  </p>

  <div v-else class="wiki-export-list">
    <template v-for="f in visibleFilenames" :key="f">
      <div class="wiki-export-row" :class="{ 'is-auto': isForced(f) }">
        <SwitchField
          controlled
          :model-value="isRowOn(f)"
          :disabled="isForced(f)"
          @update:model-value="(v) => setTemplateSelected(f, v)"
        />
        <span class="wiki-export-name">{{ displayName(f) }}</span>
        <span v-if="isForced(f)" class="wiki-export-auto">
          {{ t('workspace.information.wiki_export.auto_included_by', [forcedBy(f)]) }}
        </span>
        <span class="wiki-export-file">{{ f }}</span>
      </div>
      <div
        v-if="isTemplateSelected(f) && isPresentation(f) && decksAvailable(f).length"
        class="wiki-export-decks"
      >
        <span class="wiki-export-decks-title">
          {{ t('workspace.information.wiki_export.decks_label') }}
        </span>
        <div
          v-for="d in decksAvailable(f)"
          :key="f + '::' + d.value"
          class="wiki-export-deck-row"
        >
          <SwitchField
            controlled
            :model-value="isDeckSelected(f, d.value)"
            @update:model-value="(v) => setDeckSelected(f, d.value, v)"
          />
          <span class="wiki-export-name">{{ d.label || d.value }}</span>
        </div>
      </div>
    </template>
  </div>

  <p v-if="missingDeps.length" class="wiki-export-missing small">
    {{ t('workspace.information.wiki_export.missing_warning', [missingDeps.join(', ')]) }}
  </p>

  <div class="wiki-export-actions">
    <span class="wiki-export-count muted small">
      {{ t('workspace.information.wiki_export.selected_count', [String(selectedCount)]) }}
      <template v-if="autoCount > 0">
        {{ t('workspace.information.wiki_export.related_count', [String(autoCount)]) }}
      </template>
    </span>
    <span class="wiki-export-spacer"></span>
    <button class="tool-btn primary" :disabled="!canExport" @click="doExport">
      {{ t('workspace.information.wiki_export.export_btn') }}
    </button>
  </div>
</template>
