<script setup lang="ts">
const model = defineModel<boolean>({ default: false });

const props = withDefaults(
  defineProps<{
    onLabel?: string;
    offLabel?: string;
    disabled?: boolean;
    id?: string;
    // Controlled mode (opt-in). The default uses v-model on the native input,
    // which every existing consumer relies on. When `controlled` is set the
    // checkbox is driven purely by :checked + @change so the DOM is re-applied
    // from the source-of-truth value on each render: this matters when the
    // parent may coalesce a toggle back to its prior value (e.g. the Scoped
    // Templates list, where a native v-model would keep the optimistic flip
    // and desync). Off by default so unrelated toggles keep their exact
    // current behavior.
    controlled?: boolean;
  }>(),
  { controlled: false },
);

function onChange(e: Event): void {
  model.value = (e.target as HTMLInputElement).checked;
}
</script>

<template>
  <label class="field-switch">
    <input
      v-if="props.controlled"
      :id="id"
      type="checkbox"
      :disabled="disabled"
      :checked="model"
      @change="onChange"
    />
    <input
      v-else
      :id="id"
      type="checkbox"
      :disabled="disabled"
      v-model="model"
    />
    <span class="field-switch-track">
      <span class="field-switch-knob"></span>
    </span>
    <span v-if="onLabel || offLabel" class="field-switch-label">
      {{ model ? onLabel : offLabel }}
    </span>
  </label>
</template>
