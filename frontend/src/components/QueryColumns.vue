<script setup lang="ts">
/** QueryColumns - the Columns tab: pick and drag-reorder output columns. */
import { useI18n } from "vue-i18n";
import draggable from "vuedraggable";
import QuerySectionHead from "./QuerySectionHead.vue";
import type { QueryBuilder } from "../composables/useQueryBuilder";

const props = defineProps<{ builder: QueryBuilder }>();
const { t } = useI18n();
const b = props.builder;
</script>

<template>
  <QuerySectionHead
    :hint="t('query.columns_hint')"
    :add-label="t('query.add_column')"
    :add-disabled="b.sources.length === 0"
    @add="b.addColumn()"
  />
  <draggable
    :list="b.columns"
    tag="div"
    class="query-col-list"
    handle=".dnd-handle"
    :animation="150"
    ghost-class="dnd-ghost"
    chosen-class="dnd-chosen"
    drag-class="dnd-drag"
    item-key="id"
  >
    <template #item="{ element: c, index: i }">
      <div class="query-col-row">
        <span class="dnd-handle" aria-hidden="true">☰</span>
        <select v-model="c.sourceId" class="query-col-source" :class="{ 'query-multi': b.sourceById[c.sourceId]?.fans }">
          <option v-for="s in b.sources" :key="s.id" :value="s.id">{{ s.label }}</option>
        </select>
        <input v-model="c.header" type="text" class="query-col-header" :placeholder="b.sourceById[c.sourceId]?.label" />
        <button type="button" class="tool-btn danger" @click="b.removeColumn(i)">×</button>
      </div>
    </template>
  </draggable>
  <p v-if="!b.columns.length" class="form-description">{{ t('query.no_columns') }}</p>
</template>
