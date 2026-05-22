<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, TextField, SelectField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";
import { Service as I18nSvc } from "../../../bindings/github.com/petervdpas/formidable2/internal/modules/i18n";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

// Language list is backend-driven: each locale's own bundle declares
// its endonym (`language.endonym` key), so adding a locale is a pure
// content change - no Vue code to update. Empty list at boot degrades
// gracefully (the SelectField just shows whatever value is in config).
const languages = ref<{ value: string; label: string }[]>([]);

onMounted(async () => {
  try {
    const locs = await I18nSvc.ListLocales();
    languages.value = (locs ?? []).map((l) => ({
      value: l.code,
      label: l.endonym || l.code,
    }));
  } catch {
    languages.value = [];
  }
});
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
