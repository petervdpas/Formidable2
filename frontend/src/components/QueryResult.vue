<script setup lang="ts">
/**
 * QueryResult - presentational view of a query.Result. The parent
 * (QueryDialog) owns the data and the run lifecycle; this component only
 * renders the row count, any surfaced data-integrity anomalies, and the
 * result table in its own fixed-height, header-pinned scroll region.
 */
import { useI18n } from "vue-i18n";
import type { Result } from "../../bindings/github.com/petervdpas/formidable2/internal/modules/query";

defineProps<{ result: Result | null }>();

const { t } = useI18n();
</script>

<template>
  <section v-if="result" class="query-results">
    <p class="query-result-count">{{ t('query.result_count', { count: result.count, total: result.total }) }}</p>

    <div v-if="result.anomalies && result.anomalies.length" class="form-error query-anomalies">
      <strong>{{ t('query.anomalies', { count: result.anomalies.length }) }}</strong>
      <ul>
        <li v-for="(a, ai) in result.anomalies.slice(0, 8)" :key="ai">
          {{ t('query.anomaly_item', { value: a.value, column: a.column, expected: a.expected }) }}
        </li>
      </ul>
    </div>

    <div v-if="result.rows.length === 0" class="form-description">{{ t('query.no_results') }}</div>
    <div v-else class="query-result-scroll">
      <table class="query-table query-result-table">
        <thead>
          <tr>
            <th v-for="(h, i) in result.columns" :key="i">{{ h }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, ri) in result.rows" :key="ri">
            <td v-for="(cell, ci) in row" :key="ci">{{ cell.text }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
