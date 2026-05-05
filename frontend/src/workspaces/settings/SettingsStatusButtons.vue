<script setup lang="ts">
import { computed } from "vue";
import { FormSection, FormRow, SwitchField } from "../../components/fields";
import { useConfig } from "../../composables/useConfig";

const { config, update } = useConfig();
const cfg = computed(() => config.value!);

const gitEnabled   = computed(() => cfg.value.remote_backend === "git");
const gigotEnabled = computed(() => cfg.value.remote_backend === "gigot");

function patchButtons(partial: Record<string, unknown>) {
  return update({ status_buttons: { ...cfg.value.status_buttons, ...partial } });
}
</script>

<template>
  <p class="section-info">Enable or disable individual status buttons.</p>

  <FormSection>
    <FormRow label="Reload Button">
      <SwitchField
        :model-value="cfg.status_buttons.reloader"
        @update:model-value="(v) => patchButtons({ reloader: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Character Picker">
      <SwitchField
        :model-value="cfg.status_buttons.charpicker"
        @update:model-value="(v) => patchButtons({ charpicker: v })"
        on-label="On"
        off-label="Off"
      />
    </FormRow>
    <FormRow label="Git Quick Actions">
      <div class="row-with-badge">
        <SwitchField
          :model-value="cfg.status_buttons.gitquick"
          @update:model-value="(v) => patchButtons({ gitquick: v })"
          :disabled="!gitEnabled"
          on-label="On"
          off-label="Off"
        />
        <span v-if="!gitEnabled" class="badge badge-warn">Requires Git backend.</span>
      </div>
    </FormRow>
    <FormRow label="GiGot Load Indicator">
      <div class="row-with-badge">
        <SwitchField
          :model-value="cfg.status_buttons.gigotload"
          @update:model-value="(v) => patchButtons({ gigotload: v })"
          :disabled="!gigotEnabled"
          on-label="On"
          off-label="Off"
        />
        <span v-if="!gigotEnabled" class="badge badge-warn">Requires GiGot backend.</span>
      </div>
    </FormRow>
  </FormSection>
</template>
