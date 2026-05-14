<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField, FolderPathField } from "../fields";
import { useConfig } from "../../composables/useConfig";

// Inline form on Current Service. Mirrors GitConnection's role:
// non-secret addressing fields only. The subscription bearer lives
// in the OS keychain (account "<profile>:gigot:<repoName>") and is
// captured by the "Connect to GiGot" workspace section, not here —
// plaintext secrets do not belong in the profile JSON.
const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <FormSection v-if="cfg">
    <FormRow :label="t('settings.field.context_directory')">
      <FolderPathField
        :model-value="cfg.context_folder"
        @update:model-value="(v) => update({ context_folder: v })"
        placeholder="./Examples"
      />
    </FormRow>
    <FormRow :label="t('settings.field.gigot_base_url')">
      <TextField
        :model-value="cfg.gigot_base_url"
        @update:model-value="(v) => update({ gigot_base_url: v })"
        placeholder="https://gigot.example.com"
      />
    </FormRow>
    <FormRow :label="t('settings.field.gigot_repository')">
      <TextField
        :model-value="cfg.gigot_repo_name"
        @update:model-value="(v) => update({ gigot_repo_name: v })"
      />
    </FormRow>
  </FormSection>
</template>
