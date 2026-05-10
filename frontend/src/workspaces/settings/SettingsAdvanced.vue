<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { FormSection, FormRow, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { t } = useI18n();
const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const gitEnabled   = computed(() => cfg.value.remote_backend === "git");
const gigotEnabled = computed(() => cfg.value.remote_backend === "gigot");

function patchButtons(partial: Record<string, unknown>) {
  return update({ status_buttons: { ...cfg.value.status_buttons, ...partial } });
}
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

  <FormSection>
    <FormRow :label="t('settings.field.reload_button')">
      <SwitchField
        :model-value="cfg.status_buttons.reloader"
        @update:model-value="(v) => patchButtons({ reloader: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.character_picker')">
      <SwitchField
        :model-value="cfg.status_buttons.charpicker"
        @update:model-value="(v) => patchButtons({ charpicker: v })"
        :on-label="t('common.on')"
        :off-label="t('common.off')"
      />
    </FormRow>
    <FormRow :label="t('settings.field.git_quick_actions')">
      <div class="row-with-badge">
        <SwitchField
          :model-value="cfg.status_buttons.gitquick"
          @update:model-value="(v) => patchButtons({ gitquick: v })"
          :disabled="!gitEnabled"
          :on-label="t('common.on')"
          :off-label="t('common.off')"
        />
        <span v-if="!gitEnabled" class="badge badge-warn">{{ t('settings.requires.git_backend') }}</span>
      </div>
    </FormRow>
    <FormRow :label="t('settings.field.gigot_load_indicator')">
      <div class="row-with-badge">
        <SwitchField
          :model-value="cfg.status_buttons.gigotload"
          @update:model-value="(v) => patchButtons({ gigotload: v })"
          :disabled="!gigotEnabled"
          :on-label="t('common.on')"
          :off-label="t('common.off')"
        />
        <span v-if="!gigotEnabled" class="badge badge-warn">{{ t('settings.requires.gigot_backend') }}</span>
      </div>
    </FormRow>
  </FormSection>
</template>
