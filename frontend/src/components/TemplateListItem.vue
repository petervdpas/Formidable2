<script setup lang="ts">
const props = defineProps<{
  filename: string;
  display: string;
  active: boolean;
}>();

defineEmits<{
  (e: "pick", filename: string): void;
}>();
</script>

<template>
  <li
    :class="['sidebar-row', 'sidebar-row--stack', { active: props.active }]"
    :data-filename="props.filename"
    @click="$emit('pick', props.filename)"
  >
    <span class="template-display">{{ props.display }}</span>
    <span class="template-meta">
      <span class="badge small template-filename">{{ props.filename }}</span>
    </span>
  </li>
</template>
