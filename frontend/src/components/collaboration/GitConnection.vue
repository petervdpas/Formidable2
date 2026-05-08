<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField } from "../fields";
import { useConfig } from "../../composables/useConfig";

// Self-contained Git connection form. Reads + writes the active
// profile's git_root / git_branch fields via useConfig — same
// reactive surface SettingsLocations used to drive. Lives here (not
// inside the workspace) so future onboarding flows or modals can
// reuse the same form without duplicating field definitions.
const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <FormSection v-if="cfg">
    <FormRow :label="t('settings.field.git_root_directory')">
      <TextField
        :model-value="cfg.git_root"
        @update:model-value="(v) => update({ git_root: v })"
        placeholder="/path/to/repo"
      />
    </FormRow>
    <FormRow :label="t('settings.field.git_branch')">
      <TextField
        :model-value="cfg.git_branch"
        @update:model-value="(v) => update({ git_branch: v })"
        placeholder="main"
      />
    </FormRow>
  </FormSection>
</template>
