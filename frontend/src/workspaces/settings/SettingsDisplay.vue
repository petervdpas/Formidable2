<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, SelectField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";
import { useTheme, type ThemeId } from "../../composables/useTheme";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const { theme, setTheme } = useTheme();

const themeOptions = computed(() => [
  { value: "light",    label: t("theme.light") },
  { value: "dark",     label: t("theme.dark") },
  { value: "purplish", label: t("theme.purplish") },
] as { value: ThemeId; label: string }[]);
</script>

<template>
  <p class="section-info">{{ t('settings.display.info') }}</p>

  <FormSection>
    <FormRow :label="t('settings.field.display_theme')">
      <SelectField
        :model-value="theme"
        @update:model-value="(v) => setTheme(v as ThemeId)"
        :options="themeOptions"
      />
    </FormRow>
    <FormRow :label="t('config.use_expressions')">
      <SwitchField
        :model-value="cfg.use_expressions"
        @update:model-value="(v) => update({ use_expressions: v })"
        :on-label="t('common.show')"
        :off-label="t('common.hide')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.collapse_loop_items')">
      <SwitchField
        :model-value="cfg.loop_state_collapsed"
        @update:model-value="(v) => update({ loop_state_collapsed: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.collapse_list_table')">
      <SwitchField
        :model-value="cfg.field_state_collapsed"
        @update:model-value="(v) => update({ field_state_collapsed: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.paste_data')">
      <SwitchField
        :model-value="cfg.show_paste_buttons"
        @update:model-value="(v) => update({ show_paste_buttons: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('config.show_meta_section')">
      <SwitchField
        :model-value="cfg.show_meta_section"
        @update:model-value="(v) => update({ show_meta_section: v })"
        :on-label="t('common.show')"
        :off-label="t('common.hide')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.icon_based')">
      <SwitchField
        :model-value="cfg.show_icon_buttons"
        @update:model-value="(v) => update({ show_icon_buttons: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
  </FormSection>
</template>
