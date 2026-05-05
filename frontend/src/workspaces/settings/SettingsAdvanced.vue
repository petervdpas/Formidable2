<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, TextField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <p class="section-info">Advanced system options. Use with caution.</p>

  <FormSection>
    <FormRow label="Enable Plugins">
      <SwitchField
        :model-value="cfg.enable_plugins"
        @update:model-value="(v) => update({ enable_plugins: v })"
        on-label="Enabled"
        off-label="Disabled"
      />
    </FormRow>
    <FormRow label="Decryption Key" description="Used to decrypt sensitive fields stored in form data.">
      <TextField
        type="password"
        :model-value="cfg.encryption_key"
        @update:model-value="(v) => update({ encryption_key: v })"
      />
    </FormRow>
    <FormRow label="Development Mode">
      <SwitchField
        :model-value="cfg.development_enable"
        @update:model-value="(v) => update({ development_enable: v })"
        on-label="Enabled"
        off-label="Disabled"
      />
    </FormRow>
    <FormRow label="Enable Logging">
      <SwitchField
        :model-value="cfg.logging_enabled"
        @update:model-value="(v) => update({ logging_enabled: v })"
        on-label="Enabled"
        off-label="Disabled"
      />
    </FormRow>
  </FormSection>
</template>
