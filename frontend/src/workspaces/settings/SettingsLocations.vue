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

const isGigot = computed(() => cfg.value.remote_backend === "gigot");
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

    <FormRow :label="t('settings.field.remote_backend')">
      <SelectField
        :model-value="cfg.remote_backend"
        @update:model-value="(v) => update({ remote_backend: v })"
        :options="backends"
      />
    </FormRow>

    <FormRow v-if="isGigot" :label="t('settings.field.gigot_base_url')">
      <TextField
        :model-value="cfg.gigot_base_url"
        @update:model-value="(v) => update({ gigot_base_url: v })"
        placeholder="https://gigot.example.com"
      />
    </FormRow>
    <FormRow v-if="isGigot" :label="t('settings.field.gigot_repository')">
      <TextField
        :model-value="cfg.gigot_repo_name"
        @update:model-value="(v) => update({ gigot_repo_name: v })"
      />
    </FormRow>
    <FormRow v-if="isGigot" :label="t('settings.field.gigot_subscription_token')">
      <TextField
        type="password"
        :model-value="cfg.gigot_token"
        @update:model-value="(v) => update({ gigot_token: v })"
      />
    </FormRow>
  </FormSection>
</template>
