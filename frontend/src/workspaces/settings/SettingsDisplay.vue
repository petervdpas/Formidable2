<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, SelectField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";
import { useTheme, type ThemeId } from "../../composables/useTheme";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const { theme, setTheme } = useTheme();

const themeOptions: { value: ThemeId; label: string }[] = [
  { value: "light",    label: "Light" },
  { value: "dark",     label: "Dark" },
  { value: "purplish", label: "Purplish" },
];
</script>

<template>
  <p class="section-info">Configure the display theme and visibility of UI elements.</p>

  <FormSection>
    <FormRow label="Display Theme">
      <SelectField
        :model-value="theme"
        @update:model-value="(v) => setTheme(v as ThemeId)"
        :options="themeOptions"
      />
    </FormRow>
    <FormRow label="Expressions">
      <SwitchField
        :model-value="cfg.use_expressions"
        @update:model-value="(v) => update({ use_expressions: v })"
        on-label="Show"
        off-label="Hide"
      />
    </FormRow>
    <FormRow label="Collapse Loop Items">
      <SwitchField
        :model-value="cfg.loop_state_collapsed"
        @update:model-value="(v) => update({ loop_state_collapsed: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Collapse List/Table Fields">
      <SwitchField
        :model-value="cfg.field_state_collapsed"
        @update:model-value="(v) => update({ field_state_collapsed: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Paste-data on Lists/Tables">
      <SwitchField
        :model-value="cfg.show_paste_buttons"
        @update:model-value="(v) => update({ show_paste_buttons: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Show Meta Section">
      <SwitchField
        :model-value="cfg.show_meta_section"
        @update:model-value="(v) => update({ show_meta_section: v })"
        on-label="Show"
        off-label="Hide"
      />
    </FormRow>
    <FormRow label="Icon-based">
      <SwitchField
        :model-value="cfg.show_icon_buttons"
        @update:model-value="(v) => update({ show_icon_buttons: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
  </FormSection>
</template>
