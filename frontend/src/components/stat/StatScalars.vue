<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { StatResult } from "./types";

// Card grid for scalar_stats (min/max/sum/avg/median/stddev/count/
// percentile). Labels are i18n'd; the order is fixed and stable so the
// grid doesn't reshuffle between fields. Numbers are formatted compact
// (integers stay integers; fractions show up to 2 decimals).
const props = defineProps<{ result: StatResult | null }>();
const { t } = useI18n();

const ORDER = [
  "count",
  "sum",
  "avg",
  "median",
  "min",
  "max",
  "stddev",
  "percentile",
];

function fmt(v: number): string {
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(2).replace(/\.?0+$/, "");
}

const cards = computed(() => {
  const s = props.result?.scalars ?? {};
  return ORDER.filter((k) => k in s).map((k) => ({
    key: k,
    label: t(`workspace.stat.scalar.${k}`),
    value: fmt(s[k]),
  }));
});
</script>

<template>
  <div v-if="cards.length > 0" class="stat-scalars">
    <div v-for="c in cards" :key="c.key" class="stat-scalar-card">
      <span class="stat-scalar-value">{{ c.value }}</span>
      <span class="stat-scalar-label">{{ c.label }}</span>
    </div>
  </div>
  <p v-else class="stat-empty">No data.</p>
</template>
