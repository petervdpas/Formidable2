<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { TextField } from "./fields";

defineProps<{ modelValue: string }>();
defineEmits<{ (e: "update:modelValue", v: string): void }>();

const { t } = useI18n();
</script>

<template>
  <TextField
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    :placeholder="t('workspace.storage.tag_filter_placeholder')"
    clearable
  />
</template>
