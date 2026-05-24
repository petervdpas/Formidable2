<script setup lang="ts">
import { computed } from "vue";
import { type Grid, scalarValues, fmtNum } from "./grid";

// Rank-0 grid: one card per measure (the measure label is its own
// caption, e.g. "count", "avg(amount)").
const props = defineProps<{ grid: Grid }>();
const cards = computed(() => scalarValues(props.grid));
</script>

<template>
  <div v-if="cards.length" class="stat-scalars">
    <div v-for="c in cards" :key="c.label" class="stat-scalar-card">
      <span class="stat-scalar-value">{{ fmtNum(c.value) }}</span>
      <span class="stat-scalar-label">{{ c.label }}</span>
    </div>
  </div>
  <p v-else class="stat-empty">No data.</p>
</template>
