<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, TextField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);
</script>

<template>
  <p class="section-info">Configure the built-in server and set the listening port.</p>

  <FormSection>
    <FormRow label="Internal Server">
      <SwitchField
        :model-value="cfg.enable_internal_server"
        @update:model-value="(v) => update({ enable_internal_server: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Listening Port">
      <TextField
        type="number"
        :model-value="String(cfg.internal_server_port)"
        @update:model-value="(v) => update({ internal_server_port: Number(v) || 0 })"
      />
    </FormRow>
  </FormSection>
</template>
