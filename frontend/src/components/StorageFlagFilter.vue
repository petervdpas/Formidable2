<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { SwitchField } from "./fields";

defineProps<{ modelValue: boolean }>();
defineEmits<{ (e: "update:modelValue", v: boolean): void }>();

const { t } = useI18n();
</script>

<template>
  <SwitchField
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    :on-label="t('workspace.storage.show_all')"
    :off-label="t('workspace.storage.show_marked')"
  />
</template>
