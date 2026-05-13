<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  visible: number;
  total: number;
}>();

const { t } = useI18n();
const isFiltered = computed(() => props.visible !== props.total);
</script>

<template>
  <span class="muted small">
    <template v-if="isFiltered">
      {{ t('common.filtered_count', [visible, total]) }}
    </template>
    <template v-else>
      {{ total }}
    </template>
  </span>
</template>
