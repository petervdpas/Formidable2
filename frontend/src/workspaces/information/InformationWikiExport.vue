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
  for (const fn of selectedTemplates.value) {
    if (!isPresentation(fn)) {
      out[fn] = [];
      continue;
    }
    const avail = decksAvailable(fn);
    if (avail.length === 0) {
      out[fn] = []; // single-deck presentation
      continue;
    }
    const chosen = [...(selectedDecks.value.get(fn) ?? [])].filter((v) =>
      avail.some((d) => d.value === v),
    );
    if (chosen.length === 0) continue; // selected but no deck picked -> skip
    out[fn] = chosen;
  }
  return out;
});

const selectedCount = computed(() => Object.keys(selections.value).length);
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
      <div class="wiki-export-row">
        <SwitchField
          controlled
          :model-value="isTemplateSelected(f)"
          @update:model-value="(v) => setTemplateSelected(f, v)"
        />
        <span class="wiki-export-name">{{ displayName(f) }}</span>
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

  <div class="wiki-export-actions">
    <span class="wiki-export-count muted small">
      {{ t('workspace.information.wiki_export.selected_count', [String(selectedCount)]) }}
    </span>
    <span class="wiki-export-spacer"></span>
    <button class="tool-btn primary" :disabled="!canExport" @click="doExport">
      {{ t('workspace.information.wiki_export.export_btn') }}
    </button>
  </div>
</template>
