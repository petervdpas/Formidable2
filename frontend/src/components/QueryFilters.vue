<script setup lang="ts">
/** QueryFilters - the Filters tab: a row must match every filter. */
import { useI18n } from "vue-i18n";
import QuerySectionHead from "./QuerySectionHead.vue";
import { OP_LABEL_KEYS, type QueryBuilder } from "../composables/useQueryBuilder";

const props = defineProps<{ builder: QueryBuilder }>();
const { t } = useI18n();
const b = props.builder;
</script>

<template>
  <QuerySectionHead
    :hint="t('query.filters_hint')"
    :add-label="t('query.add_filter')"
    :add-disabled="b.sources.length === 0"
    @add="b.addFilter()"
  />
  <table v-if="b.filters.length" class="query-table">
    <thead>
      <tr>
        <th>{{ t('query.column.source') }}</th>
        <th class="query-th-narrow">{{ t('query.filter.op') }}</th>
        <th>{{ t('query.filter.value') }}</th>
        <th class="query-th-narrow"></th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="(f, i) in b.filters" :key="f.id">
        <td>
          <select v-model="f.sourceId">
            <option v-for="s in b.sources" :key="s.id" :value="s.id">{{ s.label }}</option>
          </select>
        </td>
        <td>
          <select v-model="f.op">
            <option v-for="op in b.ops" :key="op" :value="op">{{ t(OP_LABEL_KEYS[op] || op) }}</option>
          </select>
        </td>
        <td>
          <select v-if="b.sourceById[f.sourceId]?.choices" v-model="f.value">
            <option value=""></option>
            <option v-for="ch in b.sourceById[f.sourceId]!.choices" :key="ch.value" :value="ch.value">
              {{ ch.label }}
            </option>
          </select>
          <input v-else v-model="f.value" type="text" />
        </td>
        <td class="query-td-center">
          <button type="button" class="tool-btn danger" @click="b.removeFilter(i)">×</button>
        </td>
      </tr>
    </tbody>
  </table>
  <p v-else class="form-description">{{ t('query.no_filters') }}</p>
</template>
