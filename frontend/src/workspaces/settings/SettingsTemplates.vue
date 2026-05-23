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

// Virtual master switch: it owns no state. It reads "are all the
// currently-visible (search-filtered) rows on?" and flipping it drives
// the per-row toggle over exactly those visible rows. Rows hidden by the
// search are never touched.
const allVisibleOn = computed<boolean>(
  () => visibleFilenames.value.length > 0 && visibleFilenames.value.every(isEnabled),
);

// Display name for a row - prefers Template.name (user-facing), falls
// back to the bare filename when the cache hasn't filled yet or the
// template happens to have no name set.
function displayName(filename: string): string {
  const tpl = cache.value.get(filename);
  return tpl?.name?.trim() || filename;
}

async function refreshAfterToggle() {
  // Re-read the authoritative config + use-side picker, then remount the
  // rows so each switch reflects backend state (no optimistic drift).
  await reload();
  await refreshEnabled();
  rev.value++;
}

async function setTemplateEnabled(filename: string, on: boolean) {
  // EnabledTemplates is the literal visible set; the backend just
  // adds/removes the one filename. Frontend renders the reloaded state.
  await ConfigSvc.SetTemplateEnabled(filename, on);
  await refreshAfterToggle();
}

async function toggleAllVisible(on: boolean) {
  // Virtual master: drive the existing per-row toggle over the visible
  // rows, then refresh once (instead of per row) to avoid N reloads.
  for (const f of visibleFilenames.value) {
    await ConfigSvc.SetTemplateEnabled(f, on);
  }
  await refreshAfterToggle();
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

  <template v-else>
    <FormSection class="settings-templates-master">
      <FormSwitchRow
        :key="`__all__:${rev}`"
        :label="t('settings.templates.toggle_all')"
        :description="t('settings.templates.toggle_all_desc')"
        :model-value="allVisibleOn"
        @update:model-value="toggleAllVisible"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormSection>

    <FormSection>
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
</template>
