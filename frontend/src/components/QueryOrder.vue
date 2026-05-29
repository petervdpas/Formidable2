<script setup lang="ts">
/** QueryOrder - the Order tab: multi-level sort; drag to set priority. */
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
    :hint="t('query.order_hint')"
    :add-label="t('query.add_order')"
    :add-disabled="b.orderTargets.length === 0"
    @add="b.addOrder()"
  />
  <draggable
    v-if="b.orders.length"
    :list="b.orders"
    tag="div"
    class="query-col-list"
    handle=".dnd-handle"
    :animation="150"
    ghost-class="dnd-ghost"
    chosen-class="dnd-chosen"
    drag-class="dnd-drag"
    item-key="id"
  >
    <template #item="{ element: o, index: i }">
      <div class="query-col-row">
        <span class="dnd-handle" aria-hidden="true">☰</span>
        <select v-model="o.targetKey" class="query-col-source">
          <option v-for="tg in b.orderTargets" :key="tg.key" :value="tg.key">{{ tg.label }}</option>
        </select>
        <select v-model="o.desc">
          <option :value="false">{{ t('query.sort.asc') }}</option>
          <option :value="true">{{ t('query.sort.desc') }}</option>
        </select>
        <button type="button" class="tool-btn danger" @click="b.removeOrder(i)">×</button>
      </div>
    </template>
  </draggable>
  <p v-else class="form-description">{{ t('query.no_order') }}</p>
</template>
