<script setup lang="ts">
/** QueryGroup - the Group tab: tick columns to group by, add aggregates. */
import { useI18n } from "vue-i18n";
import QuerySectionHead from "./QuerySectionHead.vue";
import { AGG_FUNCS, AGG_LABEL_KEYS, needsSource, type QueryBuilder } from "../composables/useQueryBuilder";

const props = defineProps<{ builder: QueryBuilder }>();
const { t } = useI18n();
const b = props.builder;
</script>

<template>
  <QuerySectionHead :hint="t('query.group_hint')" />
  <div v-if="b.validColumns.length" class="query-col-list">
    <label v-for="c in b.validColumns" :key="c.id" class="query-col-group-row">
      <input type="checkbox" :value="c.id" v-model="b.groupDims" />
      {{ b.colLabel(c) }}
    </label>
  </div>
  <p v-else class="form-description">{{ t('query.no_columns') }}</p>

  <QuerySectionHead
    :hint="t('query.measures')"
    :add-label="t('query.add_measure')"
    :add-disabled="!b.grouping"
    @add="b.addMeasure()"
  />
  <div v-if="b.grouping && b.measures.length" class="query-col-list">
    <div v-for="(ms, i) in b.measures" :key="ms.id" class="query-col-row">
      <select v-model="ms.func">
        <option v-for="fn in AGG_FUNCS" :key="fn" :value="fn">{{ t(AGG_LABEL_KEYS[fn]) }}</option>
      </select>
      <select v-if="needsSource(ms.func)" v-model="ms.sourceId" class="query-col-source">
        <option v-for="s in b.aggregatableSources" :key="s.id" :value="s.id">{{ s.label }}</option>
      </select>
      <input v-model="ms.header" type="text" class="query-col-header" :placeholder="t(AGG_LABEL_KEYS[ms.func])" />
      <button type="button" class="tool-btn danger" @click="b.removeMeasure(i)">×</button>
    </div>
  </div>
  <p v-else class="form-description">{{ t('query.no_group') }}</p>
</template>
