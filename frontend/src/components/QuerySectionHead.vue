<script setup lang="ts">
/**
 * QuerySectionHead - the head of a query tab section: a hint plus an
 * optional "Add" button. Rendered as `.query-section-head`, which the
 * query.css rule pins to the top of the scrolling tab pane, so the Add
 * button stays reachable while the rows scroll under it. Shared by every
 * tab so the pinned-head pattern lives in one place.
 */
defineProps<{ hint: string; addLabel?: string; addDisabled?: boolean }>();
defineEmits<{ (e: "add"): void }>();
</script>

<template>
  <div class="query-section-head">
    <span class="form-description">{{ hint }}</span>
    <button v-if="addLabel" type="button" class="tool-btn" :disabled="addDisabled" @click="$emit('add')">
      {{ addLabel }}
    </button>
  </div>
</template>
