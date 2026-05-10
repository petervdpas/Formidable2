<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, FormSwitchRow, TextField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

function patchHistory(partial: Record<string, unknown>) {
  return update({ history: { ...cfg.value.history, ...partial } });
}
</script>

<template>
  <p class="section-info">{{ t('settings.history.info') }}</p>

  <FormSection>
    <FormRow :label="t('config.history.enabled')">
      <SwitchField
        :model-value="cfg.history.enabled"
        @update:model-value="(v) => patchHistory({ enabled: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormSwitchRow
      :label="t('config.history.persist')"
      :description="t('settings.desc.persist_history')"
      :model-value="cfg.history.persist"
      @update:model-value="(v) => patchHistory({ persist: v })"
      :on-label="t('common.on')"
      :off-label="t('common.off')"
    />
    <FormRow :label="t('config.history.max_size')">
      <TextField
        type="number"
        :model-value="String(cfg.history.max_size)"
        @update:model-value="(v) => patchHistory({ max_size: Number(v) || 0 })"
      />
    </FormRow>
  </FormSection>
</template>
