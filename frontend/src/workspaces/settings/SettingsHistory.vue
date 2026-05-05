<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, TextField, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);

function patchHistory(partial: Record<string, unknown>) {
  return update({ history: { ...cfg.value.history, ...partial } });
}
</script>

<template>
  <p class="section-info">Configure history settings like enabled, persist and max size.</p>

  <FormSection>
    <FormRow label="History">
      <SwitchField
        :model-value="cfg.history.enabled"
        @update:model-value="(v) => patchHistory({ enabled: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Persist History" description="Keep undo/redo history across sessions.">
      <SwitchField
        :model-value="cfg.history.persist"
        @update:model-value="(v) => patchHistory({ persist: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="History Max Size">
      <TextField
        type="number"
        :model-value="String(cfg.history.max_size)"
        @update:model-value="(v) => patchHistory({ max_size: Number(v) || 0 })"
      />
    </FormRow>
  </FormSection>
</template>
