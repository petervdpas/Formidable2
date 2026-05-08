<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField } from "../fields";
import { useConfig } from "../../composables/useConfig";

// Self-contained GiGot connection form. Same role as GitConnection
// but for the GiGot backend's three fields (base URL, repository,
// subscription token). Token uses type="password" so the value is
// masked at rest in the input.
const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <FormSection v-if="cfg">
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
    <FormRow :label="t('settings.field.gigot_subscription_token')">
      <TextField
        type="password"
        :model-value="cfg.gigot_token"
        @update:model-value="(v) => update({ gigot_token: v })"
      />
    </FormRow>
  </FormSection>
</template>
