<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, SelectField, SwitchField, TextField } from "../../components/fields";
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

// Window-size presets. "0x0" = fullscreen; main.go reads window_bounds
// on launch and applies StartState=Fullscreen when both dimensions
// are zero. Resolution numbers are technical (don't translate them);
// only the "Fullscreen" label is i18n.
type WindowPreset = { value: string; w: number; h: number; fullscreen?: boolean };
const WINDOW_PRESETS: WindowPreset[] = [
  { value: "0x0",       w: 0,    h: 0,    fullscreen: true },
  { value: "800x600",   w: 800,  h: 600 },
  { value: "1024x800",  w: 1024, h: 800 },
  { value: "1280x900",  w: 1280, h: 900 },
  { value: "1440x1000", w: 1440, h: 1000 },
  { value: "1680x1050", w: 1680, h: 1050 },
];

const currentWindowValue = computed(() => {
  const w = cfg.value?.window_bounds?.width ?? 0;
  const h = cfg.value?.window_bounds?.height ?? 0;
  return `${w}x${h}`;
});

const windowOptions = computed(() => {
  const opts = WINDOW_PRESETS.map((p) => ({
    value: p.value,
    label: p.fullscreen ? t("settings.window_preset.fullscreen") : `${p.w}×${p.h}`,
  }));
  // If the persisted value isn't a preset, surface it as a "Custom"
  // entry so the dropdown reflects reality.
  if (!WINDOW_PRESETS.some((p) => p.value === currentWindowValue.value)) {
    const w = cfg.value?.window_bounds?.width ?? 0;
    const h = cfg.value?.window_bounds?.height ?? 0;
    opts.unshift({
      value: currentWindowValue.value,
      label: t("settings.window_preset.custom_label", [w, h]),
    });
  }
  return opts;
});

function setWindowSize(value: string) {
  const preset = WINDOW_PRESETS.find((p) => p.value === value);
  if (!preset) return;
  update({
    window_bounds: {
      ...cfg.value.window_bounds,
      width: preset.w,
      height: preset.h,
    },
  });
}

const SIDEBAR_MIN = 180;
const SIDEBAR_MAX = 500;
function clampSidebar(n: number): number {
  if (Number.isNaN(n)) return SIDEBAR_MIN;
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, Math.round(n)));
}

const TOAST_MIN = 2;
const TOAST_MAX = 15;
const TOAST_DEFAULT = 5;
function clampToast(n: number): number {
  if (Number.isNaN(n)) return TOAST_DEFAULT;
  return Math.min(TOAST_MAX, Math.max(TOAST_MIN, Math.round(n)));
}

const DECIMAL_PRECISION_MIN = 0;
const DECIMAL_PRECISION_MAX = 3;
function clampDecimalPrecision(n: number): number {
  if (Number.isNaN(n)) return DECIMAL_PRECISION_MIN;
  return Math.min(DECIMAL_PRECISION_MAX, Math.max(DECIMAL_PRECISION_MIN, Math.round(n)));
}
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
    <FormRow
      :label="t('settings.field.window_size')"
      :description="t('settings.desc.window_size_apply_on_launch')"
    >
      <SelectField
        :model-value="currentWindowValue"
        @update:model-value="setWindowSize"
        :options="windowOptions"
      />
    </FormRow>
    <FormRow
      :label="t('settings.field.sidebar_width')"
      :description="t('settings.desc.sidebar_width')"
    >
      <TextField
        type="number"
        lazy
        :min="SIDEBAR_MIN"
        :max="SIDEBAR_MAX"
        :model-value="String(cfg.sidebar_width || 280)"
        @update:model-value="(v) => update({ sidebar_width: clampSidebar(Number(v)) })"
      />
    </FormRow>
    <FormRow
      :label="t('settings.field.toast_timeout')"
      :description="t('settings.desc.toast_timeout')"
    >
      <TextField
        type="number"
        lazy
        :min="TOAST_MIN"
        :max="TOAST_MAX"
        :model-value="String(cfg.toast_timeout || TOAST_DEFAULT)"
        @update:model-value="(v) => update({ toast_timeout: clampToast(Number(v)) })"
      />
    </FormRow>
    <FormRow
      :label="t('settings.field.decimal_precision')"
      :description="t('settings.desc.decimal_precision')"
    >
      <TextField
        type="number"
        lazy
        :min="DECIMAL_PRECISION_MIN"
        :max="DECIMAL_PRECISION_MAX"
        :model-value="String(cfg.decimal_precision ?? 0)"
        @update:model-value="(v) => update({ decimal_precision: clampDecimalPrecision(Number(v)) })"
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
    <FormRow :label="t('settings.field.auto_collapse_fields')">
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
    <FormRow :label="t('settings.field.sort_data')">
      <SwitchField
        :model-value="cfg.show_sort_buttons"
        @update:model-value="(v) => update({ show_sort_buttons: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.dedup_data')">
      <SwitchField
        :model-value="cfg.show_dedup_buttons"
        @update:model-value="(v) => update({ show_dedup_buttons: v })"
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
    <FormRow :label="t('config.show_copy_button')">
      <SwitchField
        :model-value="cfg.show_copy_button"
        @update:model-value="(v) => update({ show_copy_button: v })"
        :on-label="t('common.show')"
        :off-label="t('common.hide')"
      />
    </FormRow>
  </FormSection>
</template>
