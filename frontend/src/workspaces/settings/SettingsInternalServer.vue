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
  <p class="section-info">{{ t('settings.internal_server.info') }}</p>

  <FormSection>
    <FormRow :label="t('config.enable_internal_server')">
      <SwitchField
        :model-value="cfg.enable_internal_server"
        @update:model-value="(v) => update({ enable_internal_server: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.listening_port')">
      <TextField
        type="number"
        lazy
        :model-value="String(cfg.internal_server_port)"
        @update:model-value="(v) => update({ internal_server_port: Number(v) || 0 })"
      />
    </FormRow>
  </FormSection>
</template>
