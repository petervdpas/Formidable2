<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormSwitchRow, TextField } from "../../components/fields";
import { Service as ConfigSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/config";
import { useConfig } from "../../composables/useConfig";
import { useTemplates } from "../../composables/useTemplates";

const { t } = useI18n();
const { config, reload } = useConfig();
const { filenames, cache, refreshEnabled } = useTemplates();

// Bumped after every toggle so the rows remount and re-read their
// authoritative state from the reloaded config. Without it a row whose
// computed state doesn't change (the last-enabled row going to "all
// shown") keeps its optimistic switch position and desyncs.
const rev = ref(0);

const cfg = computed(() => config.value!);

// Search-box state - narrows the visible rows only. Empty string shows
// everything. Case-insensitive substring match on both filename and the
// template's display name; the user shouldn't have to remember whether
// a template is "Basic" or "basic.yaml".
const search = ref<string>("");

const visibleFilenames = computed<string[]>(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return filenames.value;
  return filenames.value.filter((f) => {
    if (f.toLowerCase().includes(q)) return true;
    const tpl = cache.value.get(f);
    const name = tpl?.name ?? "";
    return name.toLowerCase().includes(q);
  });
});

// EnabledTemplates is the literal set of visible templates. A row is on
// iff it's in the list; an empty list means nothing is visible (the
// backend treats empty the same way for the use-side picker).
const enabled = computed<string[]>(() => cfg.value.enabled_templates ?? []);
const noneEnabled = computed<boolean>(() => enabled.value.length === 0);

function isEnabled(filename: string): boolean {
  return enabled.value.includes(filename);
}

// Display name for a row - prefers Template.name (user-facing), falls
// back to the bare filename when the cache hasn't filled yet or the
// template happens to have no name set.
function displayName(filename: string): string {
  const tpl = cache.value.get(filename);
  return tpl?.name?.trim() || filename;
}

async function setTemplateEnabled(filename: string, on: boolean) {
  // Backend owns the curation logic (empty = show all, begin/extend/clear
  // scoping). The frontend just sends the toggle and re-reads the
  // authoritative config, so it renders backend state rather than
  // computing the slice itself.
  await ConfigSvc.SetTemplateEnabled(filename, on);
  await reload();
  // Re-fetch the use-side picker (StorageWorkspace) and remount the rows
  // so each switch reflects the reloaded state.
  await refreshEnabled();
  rev.value++;
}
</script>

<template>
  <p class="section-info">{{ t('settings.templates.info') }}</p>

  <p v-if="noneEnabled" class="muted small">
    {{ t('settings.templates.none_enabled') }}
  </p>

  <div class="settings-templates-search">
    <TextField
      v-model="search"
      type="text"
      :placeholder="t('settings.templates.search_placeholder')"
      clearable
    />
  </div>

  <p v-if="filenames.length === 0" class="muted small">
    {{ t('settings.templates.loading') }}
  </p>
  <p v-else-if="visibleFilenames.length === 0" class="muted small">
    {{ t('settings.templates.no_templates_found') }}
  </p>

  <FormSection v-else>
    <FormSwitchRow
      v-for="f in visibleFilenames"
      :key="`${f}:${rev}`"
      :label="displayName(f)"
      :description="f"
      :model-value="isEnabled(f)"
      @update:model-value="(v) => setTemplateEnabled(f, v)"
      :on-label="t('common.on')"
      :off-label="t('common.off')"
    />
  </FormSection>
</template>
