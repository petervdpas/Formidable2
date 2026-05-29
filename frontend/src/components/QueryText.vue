<script setup lang="ts">
/**
 * QueryText - the SQL preview tab. The text is rendered by the backend
 * (Query.Explain), so this is a dumb display of builder.sqlText; the parent
 * refreshes it when the tab is opened.
 */
import { useI18n } from "vue-i18n";
import type { QueryBuilder } from "../composables/useQueryBuilder";

const props = defineProps<{ builder: QueryBuilder }>();
const { t } = useI18n();
const b = props.builder;
</script>

<template>
  <span class="form-description">{{ t('query.sql_hint') }}</span>
  <pre v-if="b.sqlText" class="query-sql"><code>{{ b.sqlText }}</code></pre>
  <p v-else class="form-description">{{ t('query.no_columns') }}</p>
</template>
