<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <p class="section-info">{{ t('settings.advanced.info') }}</p>

  <FormSection>
    <FormRow :label="t('config.enable_plugins')">
      <SwitchField
        :model-value="cfg.enable_plugins"
        @update:model-value="(v) => update({ enable_plugins: v })"
        :on-label="t('common.enabled')"
        :off-label="t('common.disabled')"
      />
    </FormRow>
    <FormRow
      :label="t('settings.field.decryption_key')"
      :description="t('settings.desc.decryption_key')"
    >
      <TextField
        type="password"
        :model-value="cfg.encryption_key"
        @update:model-value="(v) => update({ encryption_key: v })"
      />
    </FormRow>
    <FormRow :label="t('config.development_enable')">
      <SwitchField
        :model-value="cfg.development_enable"
        @update:model-value="(v) => update({ development_enable: v })"
        :on-label="t('common.enabled')"
        :off-label="t('common.disabled')"
      />
    </FormRow>
    <FormRow :label="t('config.logging_enabled')">
      <SwitchField
        :model-value="cfg.logging_enabled"
        @update:model-value="(v) => update({ logging_enabled: v })"
        :on-label="t('common.enabled')"
        :off-label="t('common.disabled')"
      />
    </FormRow>
  </FormSection>
</template>
