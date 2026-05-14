<script setup lang="ts">
import { computed } from "vue";

// ProgressBar — generic determinate / indeterminate progress strip.
// Determinate when total > 0 and total is provided: the fill is sized
// to current/total. Indeterminate otherwise: an animated band scans
// the track until `active` flips false. Hidden entirely when inactive
// so consumers don't need to wrap it in v-if.
const props = defineProps<{
  active: boolean;
  label?: string;
  current?: number;
  total?: number;
}>();

const determinate = computed(
  () =>
    typeof props.total === "number"
    && props.total > 0
    && typeof props.current === "number",
);

const percent = computed(() => {
  if (!determinate.value) return 0;
  const c = Math.max(0, Math.min(props.current ?? 0, props.total ?? 0));
  return (c / (props.total ?? 1)) * 100;
});
</script>

<template>
  <div v-if="active" class="progress-bar" role="status" aria-live="polite">
    <div v-if="label" class="progress-bar__label">{{ label }}</div>
    <div class="progress-bar__track">
      <div
        v-if="determinate"
        class="progress-bar__fill"
        :style="{ width: percent + '%' }"
      ></div>
      <div v-else class="progress-bar__fill progress-bar__fill--indeterminate"></div>
    </div>
    <div v-if="determinate" class="progress-bar__count">
      {{ current }} / {{ total }}
    </div>
  </div>
</template>
