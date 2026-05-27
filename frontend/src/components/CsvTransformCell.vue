<script setup lang="ts">
import { useI18n } from "vue-i18n";
import {
  transformRules,
  transformLabelKey,
  paramPlaceholder,
  paramInputType,
} from "../utils/csvTransforms";

const rule = defineModel<string>("rule", { required: true });
const param = defineModel<string>("param", { required: true });

const { t } = useI18n();
</script>

<template>
  <div class="csv-transform-cell">
    <select v-model="rule">
      <option v-for="r in transformRules" :key="r" :value="r">
        {{ t(transformLabelKey[r]) }}
      </option>
    </select>
    <input
      v-if="paramPlaceholder[rule] !== undefined"
      :type="paramInputType[rule] ?? 'text'"
      :placeholder="rule === 'bool-match' ? t('csv.transform.boolmatch.placeholder') : paramPlaceholder[rule]"
      v-model="param"
      class="csv-import-param"
    />
  </div>
</template>
