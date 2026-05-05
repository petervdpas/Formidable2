<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

// Endonyms — language names stay in their own language so users find
// their language even if the UI is in the "wrong" one.
const languages = [
  { value: "en", label: "English" },
  { value: "nl", label: "Nederlands" },
];
</script>

<template>
  <p class="section-info">{{ t('settings.general.info') }}</p>

  <FormSection>
    <FormRow :label="t('config.profile_name')">
      <TextField
        :model-value="cfg.profile_name"
        @update:model-value="(v) => update({ profile_name: v })"
      />
    </FormRow>
    <FormRow :label="t('config.language')">
      <SelectField
        :model-value="cfg.language"
        @update:model-value="(v) => update({ language: v })"
        :options="languages"
      />
    </FormRow>
    <FormRow :label="t('config.author_name')">
      <TextField
        :model-value="cfg.author_name"
        @update:model-value="(v) => update({ author_name: v })"
      />
    </FormRow>
    <FormRow :label="t('config.author_email')">
      <TextField
        type="email"
        :model-value="cfg.author_email"
        @update:model-value="(v) => update({ author_email: v })"
      />
    </FormRow>
  </FormSection>
</template>
