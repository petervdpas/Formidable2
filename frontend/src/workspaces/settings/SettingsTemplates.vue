<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormSwitchRow, TextField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";
import { useTemplates } from "../../composables/useTemplates";

const { t } = useI18n();
const { config, update } = useConfig();
const { filenames, cache, refreshEnabled } = useTemplates();

const cfg = computed(() => config.value!);

// Search-box state — narrows the visible rows only. Empty string shows
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

// The opt-in semantic: empty/nil EnabledTemplates means "all enabled".
// Reflected back to the toggle: every row reads as "on" until the user
// flips one, at which point the slice becomes authoritative.
const enabled = computed<string[]>(() => cfg.value.enabled_templates ?? []);
const optedIn = computed<boolean>(() => enabled.value.length > 0);

function isEnabled(filename: string): boolean {
  if (!optedIn.value) return true;
  return enabled.value.includes(filename);
}

// Display name for a row — prefers Template.name (user-facing), falls
// back to the bare filename when the cache hasn't filled yet or the
// template happens to have no name set.
function displayName(filename: string): string {
  const tpl = cache.value.get(filename);
  return tpl?.name?.trim() || filename;
}

async function setTemplateEnabled(filename: string, on: boolean) {
  // Compute the post-toggle slice. Two semantics meet here:
  //   - "Opting in for the first time" — empty list, user turns ONE on.
  //     We seed with everything that was implicitly on (all filenames)
  //     EXCEPT the one being turned off... but the only path through
  //     this branch is on=true, since on=false on an empty list is a
  //     visible toggle change for a row that was reading as "on".
  //   - "Curating an existing list" — non-empty, user adds/removes one.
  //
  // The first-toggle case is the subtle one: clicking "off" on a row
  // when nothing is opted in means "I want everything EXCEPT this one"
  // — so we seed from filenames minus this one. Clicking "on" with
  // nothing opted in is meaningless (everything was on already), but
  // we still write the single entry so the UI reflects the user's
  // intent and the list becomes authoritative.
  let next: string[];
  if (!optedIn.value) {
    if (on) {
      next = [filename];
    } else {
      next = filenames.value.filter((f) => f !== filename);
    }
  } else if (on) {
    next = [...enabled.value, filename];
  } else {
    next = enabled.value.filter((f) => f !== filename);
  }
  await update({ enabled_templates: next });
  // Tell the use-side picker (StorageWorkspace) to re-fetch from the
  // backend so the new curation is live without a page reload.
  await refreshEnabled();
}
</script>

<template>
  <p class="section-info">{{ t('settings.templates.info') }}</p>

  <p v-if="!optedIn" class="muted small">
    {{ t('settings.templates.empty_means_all') }}
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
      :key="f"
      :label="displayName(f)"
      :description="f"
      :model-value="isEnabled(f)"
      @update:model-value="(v) => setTemplateEnabled(f, v)"
      :on-label="t('common.on')"
      :off-label="t('common.off')"
    />
  </FormSection>
</template>
