<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const backends = computed(() => [
  { value: "none",  label: t("backend.none") },
  { value: "git",   label: t("backend.git") },
  { value: "gigot", label: t("backend.gigot") },
]);
</script>

<template>
  <p class="section-info">{{ t('settings.locations.info') }}</p>

  <FormSection>
    <FormRow
      :label="t('settings.field.context_directory')"
      :description="t('settings.desc.context_directory')"
    >
      <TextField
        :model-value="cfg.context_folder"
        @update:model-value="(v) => update({ context_folder: v })"
        placeholder="/path/to/context"
      />
    </FormRow>

    <FormRow
      :label="t('settings.field.remote_backend')"
      :description="t('settings.desc.remote_backend')"
    >
      <SelectField
        :model-value="cfg.remote_backend"
        @update:model-value="(v) => update({ remote_backend: v })"
        :options="backends"
      />
    </FormRow>
  </FormSection>
</template>
